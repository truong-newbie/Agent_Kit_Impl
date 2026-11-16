package dist

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestCreateKitCreateSkillAndDeterministicPack(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := CreateKit(base, "acme-eng", "Acme Engineering")
	if err != nil {
		t.Fatalf("CreateKit failed: %v", err)
	}
	if _, err := CreateSkill(created.Dir, "audit", "Audit security boundaries"); err != nil {
		t.Fatalf("CreateSkill failed: %v", err)
	}

	outA := filepath.Join(base, "outA", "acme.tar.gz")
	outB := filepath.Join(base, "outB", "acme.tar.gz")
	resA, err := Pack(created.Dir, outA)
	if err != nil {
		t.Fatalf("Pack A failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	resB, err := Pack(created.Dir, outB)
	if err != nil {
		t.Fatalf("Pack B failed: %v", err)
	}
	if resA.ArchiveSHA256 != resB.ArchiveSHA256 || resA.PackageDigest != resB.PackageDigest {
		t.Fatalf("non-deterministic pack: %+v vs %+v", resA, resB)
	}

	bytesA, _ := os.ReadFile(outA)
	bytesB, _ := os.ReadFile(outB)
	if !bytes.Equal(bytesA, bytesB) {
		t.Fatal("packed tar.gz bytes differ across runs")
	}

	pkg, _, dig, sha, err := ReadArchiveFile(outA, resA.ArchiveSHA256)
	if err != nil || pkg.ID != "acme-eng" || dig != resA.PackageDigest || sha != resA.ArchiveSHA256 {
		t.Fatalf("ReadArchiveFile failed: pkg=%+v dig=%s sha=%s err=%v", pkg, dig, sha, err)
	}
}

func makeRawTarGz(t *testing.T, fn func(*tar.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	fn(tw)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestReadArchiveRejectsHostileArchives(t *testing.T) {
	// 1. Path traversal
	trav := makeRawTarGz(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "../escape.txt", Mode: 0644, Size: 1, Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte("x"))
	})
	if _, _, _, _, err := ReadArchiveBytes(trav, ""); err == nil {
		t.Fatal("accepted path traversal archive")
	}

	// 2. Symlink
	sym := makeRawTarGz(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "rules/link.md", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	})
	if _, _, _, _, err := ReadArchiveBytes(sym, ""); err == nil {
		t.Fatal("accepted symlink archive")
	}

	// 3. Hardlink
	hard := makeRawTarGz(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "rules/hard.md", Typeflag: tar.TypeLink, Linkname: "package.json"})
	})
	if _, _, _, _, err := ReadArchiveBytes(hard, ""); err == nil {
		t.Fatal("accepted hardlink archive")
	}

	// 4. Device / FIFO
	fifo := makeRawTarGz(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "rules/pipe", Typeflag: tar.TypeFifo})
	})
	if _, _, _, _, err := ReadArchiveBytes(fifo, ""); err == nil {
		t.Fatal("accepted FIFO archive entry")
	}

	// 5. Case collision
	coll := makeRawTarGz(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "rules/A.md", Mode: 0644, Size: 1, Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte("a"))
		_ = tw.WriteHeader(&tar.Header{Name: "rules/a.md", Mode: 0644, Size: 1, Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte("b"))
	})
	if _, _, _, _, err := ReadArchiveBytes(coll, ""); err == nil {
		t.Fatal("accepted case-colliding archive")
	}

	// 6. Executable script inside archive
	script := makeRawTarGz(t, func(tw *tar.Writer) {
		_ = tw.WriteHeader(&tar.Header{Name: "run.sh", Mode: 0644, Size: 9, Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte("#!/bin/sh"))
	})
	if _, _, _, _, err := ReadArchiveBytes(script, ""); err == nil {
		t.Fatal("accepted script in archive")
	}

	// 7. Oversized tar stream / zip-bomb guard (> 1 MiB single file or > 10 MiB total)
	big := makeRawTarGz(t, func(tw *tar.Writer) {
		payload := bytes.Repeat([]byte("a"), catalog.MaxFileBytes+5)
		_ = tw.WriteHeader(&tar.Header{Name: "rules/big.md", Mode: 0644, Size: int64(len(payload)), Typeflag: tar.TypeReg})
		_, _ = tw.Write(payload)
	})
	if _, _, _, _, err := ReadArchiveBytes(big, ""); err == nil {
		t.Fatal("accepted oversized file in archive")
	}
}

func TestRemoteRegistryCacheCorruptionAndOfflineMode(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	kitDir, err := CreateKit(base, "remote-kit", "Remote Kit")
	if err != nil {
		t.Fatal(err)
	}
	packRes, err := Pack(kitDir.Dir, filepath.Join(base, "remote-kit-0.1.0.tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	archiveBytes, err := os.ReadFile(packRes.ArchivePath)
	if err != nil {
		t.Fatal(err)
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			idx := RegistryIndex{
				SchemaVersion: 1,
				Packages: []IndexEntry{{
					ID:            "remote-kit",
					Version:       "0.1.0",
					URL:           server.URL + "/remote-kit-0.1.0.tar.gz",
					ArchiveSHA256: packRes.ArchiveSHA256,
					PackageDigest: packRes.PackageDigest,
					MinCLIVersion: "0.1.0",
				}},
			}
			_ = json.NewEncoder(w).Encode(idx)
		case "/remote-kit-0.1.0.tar.gz":
			_, _ = w.Write(archiveBytes)
		case "/broken.tar.gz":
			_, _ = w.Write([]byte("partial truncated"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cache := Cache{Root: filepath.Join(base, "cache")}

	// Reject plain non-loopback HTTP.
	if err := ValidateRemoteURL("http://example.com/index.json"); apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected ExitUsage for non-loopback http URL, got %v", err)
	}

	// Resolve from remote registry index and populate cache.
	resolved, err := ResolveRemote(context.Background(), server.Client(), cache, "remote-kit", "0.1.0", server.URL+"/index.json", "", "0.1.0-dev", false)
	if err != nil || resolved.Package.ID != "remote-kit" || resolved.ArchiveSHA256 != packRes.ArchiveSHA256 {
		t.Fatalf("ResolveRemote failed: %+v err=%v", resolved, err)
	}

	// Offline reinstall succeeds purely from cache without contacting server.
	server.Close()
	offlineRes, err := ResolveRemote(context.Background(), nil, cache, "remote-kit", "0.1.0", "", packRes.ArchiveSHA256, "0.1.0-dev", true)
	if err != nil || offlineRes.PackageDigest != packRes.PackageDigest {
		t.Fatalf("offline cache resolve failed: %+v err=%v", offlineRes, err)
	}

	// Cache corruption must be detected and rejected.
	cachedFile, err := cache.archivePath(packRes.ArchiveSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachedFile, []byte("corrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveRemote(context.Background(), nil, cache, "remote-kit", "0.1.0", "", packRes.ArchiveSHA256, "0.1.0-dev", true); apperr.ExitCodeOf(err) != apperr.ExitConflict || !strings.Contains(err.Error(), "corrupted") {
		t.Fatalf("expected cache_corrupted conflict, got %v", err)
	}

	// CleanStaleDownloads removes leftover .part files inside cache/tmp.
	tmpDir := filepath.Join(cache.Root, "tmp")
	_ = os.MkdirAll(tmpDir, 0700)
	stalePart := filepath.Join(tmpDir, "dl-interrupted.part")
	_ = os.WriteFile(stalePart, []byte("incomplete"), 0600)
	if err := cache.CleanStaleDownloads(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stalePart); !os.IsNotExist(err) {
		t.Fatalf("stale .part file survived cleanup: %v", err)
	}

	// Incompatible minCliVersion must be rejected clearly.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(RegistryIndex{
			SchemaVersion: 1,
			Packages: []IndexEntry{{
				ID:            "remote-kit",
				Version:       "0.1.0",
				URL:           "https://example.invalid/remote-kit-0.1.0.tar.gz",
				ArchiveSHA256: packRes.ArchiveSHA256,
				MinCLIVersion: "9.0.0",
			}},
		})
	}))
	defer srv2.Close()
	if _, err := ResolveRemote(context.Background(), srv2.Client(), cache, "remote-kit", "0.1.0", srv2.URL+"/index.json", "", "0.1.0-dev", false); apperr.ExitCodeOf(err) != apperr.ExitUsage || !strings.Contains(err.Error(), "requires dtkit >=") {
		t.Fatalf("expected cli_upgrade_required usage error, got %v", err)
	}
}
