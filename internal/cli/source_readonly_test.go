package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
)

type cliTrapTransport struct {
	calls atomic.Int32
}

func (tr *cliTrapTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	tr.calls.Add(1)
	return nil, errors.New("network access forbidden in offline/local/cache-hit mode")
}

type fsEntrySnapshot struct {
	IsDir     bool
	Mode      fs.FileMode
	Size      int64
	SHA256    string
	ModTimeNs int64
}

func snapshotRoot(t *testing.T, root string) map[string]fsEntrySnapshot {
	t.Helper()
	out := map[string]fsEntrySnapshot{}
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
		rec := fsEntrySnapshot{
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
		t.Fatalf("snapshotRoot(%s): %v", root, err)
	}
	return out
}

func makeRemoteKitArchive(t *testing.T, dir, version string) (dist.PackResult, []byte) {
	t.Helper()
	created, err := dist.CreateKit(dir, "remote-eng", "Remote Engineering")
	if err != nil {
		t.Fatal(err)
	}
	pkgPath := filepath.Join(created.Dir, "package.json")
	rawPkg, err := os.ReadFile(pkgPath)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(rawPkg, &m); err != nil {
		t.Fatal(err)
	}
	m["version"] = version
	updatedPkg, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pkgPath, append(updatedPkg, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	if version != "0.1.0" {
		if err := os.WriteFile(filepath.Join(created.Dir, "rules", "engineering.md"), []byte("# Engineering v"+version+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	packRes, err := dist.Pack(created.Dir, filepath.Join(dir, "remote-eng-"+version+".tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	archiveBytes, err := os.ReadFile(packRes.ArchivePath)
	if err != nil {
		t.Fatal(err)
	}
	return packRes, archiveBytes
}

func TestCLIReadOnlyContractAcrossDryRunInfoValidateOfflineAndCancel(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pack1, bytes1 := makeRemoteKitArchive(t, filepath.Join(base, "src-v1"), "0.1.0")
	pack2, bytes2 := makeRemoteKitArchive(t, filepath.Join(base, "src-v2"), "0.2.0")

	var indexMode atomic.Int32 // 1 = v1 only, 2 = v1+v2
	indexMode.Store(1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			pkgs := []dist.IndexEntry{{
				ID:            "remote-eng",
				Version:       "0.1.0",
				URL:           "/remote-eng-0.1.0.tar.gz",
				ArchiveSHA256: pack1.ArchiveSHA256,
				PackageDigest: pack1.PackageDigest,
			}}
			if indexMode.Load() == 2 {
				pkgs = append(pkgs, dist.IndexEntry{
					ID:            "remote-eng",
					Version:       "0.2.0",
					URL:           "/remote-eng-0.2.0.tar.gz",
					ArchiveSHA256: pack2.ArchiveSHA256,
					PackageDigest: pack2.PackageDigest,
				})
			}
			_ = json.NewEncoder(w).Encode(dist.RegistryIndex{SchemaVersion: 1, Packages: pkgs})
		case "/remote-eng-0.1.0.tar.gz":
			_, _ = w.Write(bytes1)
		case "/remote-eng-0.2.0.tar.gz":
			_, _ = w.Write(bytes2)
		case "/corrupt.tar.gz":
			_, _ = w.Write([]byte("not-a-valid-gzip"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	workspace := filepath.Join(base, "workspace")
	proj := filepath.Join(workspace, "project")
	home := filepath.Join(workspace, "home")
	codexHome := filepath.Join(home, ".codex")
	cacheBase := filepath.Join(workspace, "xdg-cache")
	stalePart := filepath.Join(cacheBase, "dtkit", "tmp", "dl-other-process.part")
	for _, d := range []string{proj, codexHome, filepath.Dir(stalePart)} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(stalePart, []byte("other-process-partial"), 0600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stalePart, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	runCmd := func(stdin string, interactive bool, client *http.Client, args ...string) (string, int, error) {
		d, out, _ := testDeps(stdin, interactive, home, codexHome, proj, true)
		d.Env.UserCacheDir = func() (string, error) { return cacheBase, nil }
		d.HTTPClient = client
		err := RunWithDeps(t.Context(), args, d)
		return out.String(), apperr.ExitCodeOf(err), err
	}

	beforeAll := snapshotRoot(t, workspace)

	// A. install --dry-run (remote cache miss) writes zero files anywhere and claims "Nothing was written."
	stdout, code, err := runCmd("", false, srv.Client(), "install", "remote-eng", "--source", srv.URL+"/index.json", "--project", "--dry-run")
	if err != nil || code != 0 || !strings.Contains(stdout, "Nothing was written.") || !reflect.DeepEqual(beforeAll, snapshotRoot(t, workspace)) {
		t.Fatalf("dry-run miss mutated workspace or failed: code=%d err=%v out=%s", code, err, stdout)
	}

	// B. info and validate against remote source do zero writes/cleanup.
	stdout, code, err = runCmd("", false, srv.Client(), "info", "remote-eng", "--source", srv.URL+"/index.json", "--json")
	if err != nil || code != 0 || !reflect.DeepEqual(beforeAll, snapshotRoot(t, workspace)) {
		t.Fatalf("info remote mutated workspace or failed: code=%d err=%v out=%s", code, err, stdout)
	}
	stdout, code, err = runCmd("", false, srv.Client(), "validate", "remote-eng", "--source", srv.URL+"/index.json", "--json")
	if err != nil || code != 0 || !reflect.DeepEqual(beforeAll, snapshotRoot(t, workspace)) {
		t.Fatalf("validate remote mutated workspace or failed: code=%d err=%v out=%s", code, err, stdout)
	}

	// C. Missing --yes (non-interactive) and cancelled interactive prompt write zero files (including cache).
	_, code, _ = runCmd("", false, srv.Client(), "install", "remote-eng", "--source", srv.URL+"/index.json", "--project")
	if code != apperr.ExitUsage || !reflect.DeepEqual(beforeAll, snapshotRoot(t, workspace)) {
		t.Fatalf("missing --yes mutated workspace: code=%d", code)
	}
	_, code, _ = runCmd("n\n", true, srv.Client(), "install", "remote-eng", "--source", srv.URL+"/index.json", "--project")
	if code != apperr.ExitCancelled || !reflect.DeepEqual(beforeAll, snapshotRoot(t, workspace)) {
		t.Fatalf("cancelled install mutated workspace: code=%d", code)
	}

	// D. Invalid remote sources fail cleanly with zero writes.
	for _, badArgs := range [][]string{
		{"install", "remote-eng", "--source", srv.URL + "/missing.json", "--project", "--yes"},
		{"install", "remote-eng", "--source", srv.URL + "/corrupt.tar.gz", "--project", "--yes"},
		{"install", "remote-eng", "--source", "http://example.com/index.json", "--project", "--yes"},
		{"install", "remote-eng", "--offline", "--sha256", pack1.ArchiveSHA256, "--project", "--yes"},
	} {
		if _, badCode, _ := runCmd("", false, srv.Client(), badArgs...); badCode == 0 {
			t.Fatalf("expected failure for %v", badArgs)
		}
		if !reflect.DeepEqual(beforeAll, snapshotRoot(t, workspace)) {
			t.Fatalf("invalid source %v mutated workspace", badArgs)
		}
	}

	// E. Confirmed remote install persists cache AND installs project artifacts (reusing exact planned snapshot).
	if stdout, code, err = runCmd("", false, srv.Client(), "install", "remote-eng", "--source", srv.URL+"/index.json", "--project", "--yes", "--json"); err != nil || code != 0 {
		t.Fatalf("confirmed remote install failed: code=%d err=%v out=%s", code, err, stdout)
	}
	if _, err := os.Stat(stalePart); err != nil {
		t.Fatalf("confirmed install cleaned unrelated .part file %s: %v", stalePart, err)
	}
	cachedV1 := filepath.Join(cacheBase, "dtkit", "packages", "sha256", pack1.ArchiveSHA256+".tar.gz")
	if err := os.Chtimes(cachedV1, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	afterV1Install := snapshotRoot(t, workspace)

	// F. Dry-run cache hit, no-op repeat install, offline info/validate/dry-run MUST NOT rewrite cache blob mtime or touch HTTP trap!
	trap := &cliTrapTransport{}
	trapClient := &http.Client{Transport: trap}
	for _, roArgs := range [][]string{
		{"info", "remote-eng", "--offline", "--sha256", pack1.ArchiveSHA256, "--json"},
		{"validate", "remote-eng", "--offline", "--sha256", pack1.ArchiveSHA256, "--json"},
		{"install", "remote-eng", "--offline", "--sha256", pack1.ArchiveSHA256, "--project", "--dry-run"},
		{"install", "remote-eng", "--offline", "--sha256", pack1.ArchiveSHA256, "--project", "--yes", "--json"},
		{"update", "remote-eng", "--offline", "--sha256", pack1.ArchiveSHA256, "--project", "--yes", "--json"},
	} {
		stdout, code, err = runCmd("", false, trapClient, roArgs...)
		if err != nil || code != 0 {
			t.Fatalf("trap/cache-hit command %v failed: code=%d err=%v out=%s", roArgs, code, err, stdout)
		}
		if !reflect.DeepEqual(afterV1Install, snapshotRoot(t, workspace)) {
			t.Fatalf("cache hit/noop/offline command %v mutated files or rewrote cached blob timestamp", roArgs)
		}
	}
	if got := trap.calls.Load(); got != 0 {
		t.Fatalf("expected 0 HTTP requests in offline/cache-hit mode, got %d", got)
	}

	// G. Remote update to v0.2.0: --dry-run (miss) writes nothing; cancelled prompt writes nothing; --yes updates & caches v0.2.0.
	indexMode.Store(2)
	if stdout, code, err = runCmd("", false, srv.Client(), "update", "remote-eng", "--source", srv.URL+"/index.json", "--project", "--dry-run"); err != nil || code != 0 || !strings.Contains(stdout, "Nothing was written.") || !reflect.DeepEqual(afterV1Install, snapshotRoot(t, workspace)) {
		t.Fatalf("update dry-run miss mutated state/cache: code=%d err=%v out=%s", code, err, stdout)
	}
	if _, code, _ = runCmd("n\n", true, srv.Client(), "update", "remote-eng", "--source", srv.URL+"/index.json", "--project"); code != apperr.ExitCancelled || !reflect.DeepEqual(afterV1Install, snapshotRoot(t, workspace)) {
		t.Fatalf("cancelled update mutated state/cache: code=%d", code)
	}
	if stdout, code, err = runCmd("", false, srv.Client(), "update", "remote-eng", "--source", srv.URL+"/index.json", "--project", "--yes", "--json"); err != nil || code != 0 {
		t.Fatalf("confirmed remote update failed: code=%d err=%v out=%s", code, err, stdout)
	}
	cachedV2 := filepath.Join(cacheBase, "dtkit", "packages", "sha256", pack2.ArchiveSHA256+".tar.gz")
	if _, err := os.Stat(cachedV2); err != nil {
		t.Fatalf("expected v0.2.0 archive cached after confirmed update: %v", err)
	}
}
