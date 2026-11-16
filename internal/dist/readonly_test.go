package dist

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type trapRoundTripper struct {
	t *testing.T
}

func (tr trapRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	tr.t.Helper()
	return nil, fmt.Errorf("unexpected HTTP request %s %s in offline/cached mode", r.Method, r.URL.String())
}

type fileRecord struct {
	IsDir     bool
	Mode      fs.FileMode
	Size      int64
	SHA256    string
	ModTimeNs int64
}

func snapshotDirTree(t *testing.T, root string) map[string]fileRecord {
	t.Helper()
	out := map[string]fileRecord{}
	if _, err := os.Lstat(root); os.IsNotExist(err) {
		return out
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rec := fileRecord{
			IsDir:     info.IsDir(),
			Mode:      info.Mode(),
			Size:      info.Size(),
			ModTimeNs: info.ModTime().UnixNano(),
		}
		if !info.IsDir() {
			raw, rErr := os.ReadFile(path)
			if rErr != nil {
				return rErr
			}
			sum := sha256.Sum256(raw)
			rec.SHA256 = hex.EncodeToString(sum[:])
		}
		out[filepath.ToSlash(rel)] = rec
		return nil
	})
	if err != nil {
		t.Fatalf("snapshotDirTree(%s): %v", root, err)
	}
	return out
}

func TestResolveRemoteReadOnlyNeverWritesOrCleansCache(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	kitDir := filepath.Join(base, "kit")
	created, err := CreateKit(kitDir, "ro-kit", "ReadOnly Kit")
	if err != nil {
		t.Fatal(err)
	}
	packRes, err := Pack(created.Dir, filepath.Join(base, "ro-kit-0.1.0.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	archiveBytes, err := os.ReadFile(packRes.ArchivePath)
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			_ = json.NewEncoder(w).Encode(RegistryIndex{
				SchemaVersion: 1,
				Packages: []IndexEntry{{
					ID:            "ro-kit",
					Version:       "0.1.0",
					URL:           "/ro-kit-0.1.0.tar.gz",
					ArchiveSHA256: packRes.ArchiveSHA256,
					PackageDigest: packRes.PackageDigest,
					MinCLIVersion: "0.1.0",
				}},
			})
		case "/ro-kit-0.1.0.tar.gz":
			_, _ = w.Write(archiveBytes)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Seed cache tmp with active & stale .part files before read-only resolution.
	cacheRoot := filepath.Join(base, "cache")
	tmpDir := filepath.Join(cacheRoot, "tmp")
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		t.Fatal(err)
	}
	activePart := filepath.Join(tmpDir, "dl-recent.part")
	liveLockedPart := filepath.Join(tmpDir, "dl-live-pid.part")
	stalePart := filepath.Join(tmpDir, "dl-old.part")
	deadLockedPart := filepath.Join(tmpDir, "dl-dead-pid.part")
	for _, p := range []string{activePart, liveLockedPart, stalePart, deadLockedPart} {
		if err := os.WriteFile(p, []byte("part-data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(liveLockedPart+".lock", []byte(fmt.Sprintf(`{"pid":%d}`, os.Getpid())), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deadLockedPart+".lock", []byte(`{"pid":9999999}`), 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stalePart, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	var writeCount, cleanCount int
	cache := Cache{
		Root: cacheRoot,
		Hooks: &CacheHooks{
			OnWrite: func(string, bool) { writeCount++ },
			OnClean: func(string) { cleanCount++ },
		},
	}

	before := snapshotDirTree(t, cacheRoot)

	// 1. ReadOnly resolve via index URL on cache miss: zero disk writes, zero cleanup.
	roIdx, err := ResolveRemote(context.Background(), srv.Client(), cache, "ro-kit", "0.1.0", srv.URL+"/index.json", "", "0.4.0-dev", false, CacheReadOnly)
	if err != nil {
		t.Fatalf("CacheReadOnly index resolve failed: %v", err)
	}
	if !roIdx.HasPendingCacheWrite() {
		t.Fatalf("expected HasPendingCacheWrite on cache miss in CacheReadOnly mode")
	}

	// 2. ReadOnly resolve via direct archive URL on cache miss: zero disk writes, zero cleanup.
	roArch, err := ResolveRemote(context.Background(), srv.Client(), cache, "ro-kit", "0.1.0", srv.URL+"/ro-kit-0.1.0.tar.gz", packRes.ArchiveSHA256, "0.4.0-dev", false, CacheReadOnly)
	if err != nil {
		t.Fatalf("CacheReadOnly archive resolve failed: %v", err)
	}
	if !roArch.HasPendingCacheWrite() {
		t.Fatalf("expected HasPendingCacheWrite on direct archive cache miss in CacheReadOnly mode")
	}

	if writeCount != 0 || cleanCount != 0 {
		t.Fatalf("expected zero cache writes/cleanups in read-only mode, got writes=%d cleans=%d", writeCount, cleanCount)
	}
	if after := snapshotDirTree(t, cacheRoot); !reflect.DeepEqual(before, after) {
		t.Fatalf("cache filesystem mutated during CacheReadOnly resolution:\nbefore=%+v\nafter=%+v", before, after)
	}

	// 3. Explicit confirmed PersistCache stores blob atomically, leaving all .part fixtures untouched.
	wrote, err := roIdx.PersistCache(cache)
	if err != nil || !wrote {
		t.Fatalf("PersistCache failed: wrote=%v err=%v", wrote, err)
	}
	cachedPath, err := cache.archivePath(packRes.ArchiveSHA256)
	if err != nil {
		t.Fatal(err)
	}
	// Backdate cached archive mtime to verify subsequent hits / no-ops never touch or rewrite it.
	if err := os.Chtimes(cachedPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	afterPersist := snapshotDirTree(t, cacheRoot)

	// 4. Repeat PersistCache (no-op) must not rewrite or touch timestamp.
	wroteAgain, err := roIdx.PersistCache(cache)
	if err != nil || wroteAgain {
		t.Fatalf("expected no-op PersistCache when already cached, got wrote=%v err=%v", wroteAgain, err)
	}

	// 5. Offline resolve with trap HTTP client must never touch network or cache files.
	trapClient := &http.Client{Transport: trapRoundTripper{t: t}}
	offRes, err := ResolveRemote(context.Background(), trapClient, cache, "ro-kit", "0.1.0", srv.URL+"/index.json", packRes.ArchiveSHA256, "0.4.0-dev", true, CacheReadOnly)
	if err != nil || offRes.ArchiveSHA256 != packRes.ArchiveSHA256 {
		t.Fatalf("offline cache hit failed: %+v err=%v", offRes, err)
	}
	if _, err := ResolveRemote(context.Background(), trapClient, cache, "ro-kit", "0.1.0", srv.URL+"/index.json", strings.Repeat("0", 64), "0.4.0-dev", true, CacheReadOnly); err == nil {
		t.Fatalf("expected offline cache miss error")
	}
	if afterOffline := snapshotDirTree(t, cacheRoot); !reflect.DeepEqual(afterPersist, afterOffline) {
		t.Fatalf("offline resolution mutated cache or rewrote blob mtime:\nbefore=%+v\nafter=%+v", afterPersist, afterOffline)
	}

	// 6. Explicit CleanStaleDownloads preserves active/live .part files and removes only stale/dead .part files.
	if err := cache.CleanStaleDownloads(); err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{activePart, liveLockedPart, cachedPath} {
		if _, err := os.Stat(keep); err != nil {
			t.Fatalf("expected active/cached file %s to survive GC: %v", keep, err)
		}
	}
	for _, gone := range []string{stalePart, deadLockedPart, deadLockedPart + ".lock"} {
		if _, err := os.Stat(gone); !os.IsNotExist(err) {
			t.Fatalf("expected stale file %s to be cleaned, got err=%v", gone, err)
		}
	}
}
