package cli

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
)

func TestAuthoringPackInstallUpdateOfflineAndRemoveFlow(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authorRoot := filepath.Join(base, "author")
	proj := filepath.Join(base, "proj")
	home := filepath.Join(base, "home")
	cacheDir := filepath.Join(base, "cache")
	for _, d := range []string{authorRoot, proj, home, cacheDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	deps, out, _ := testDeps("", false, home, "", proj, true)
	deps.Env.UserCacheDir = func() (string, error) { return cacheDir, nil }

	// 1. Author creates a new kit + skill + validates + packs v0.1.0 without touching CLI source.
	if err := RunWithDeps(context.Background(), []string{"create", "kit", "acme", "--dir", authorRoot, "--json"}, deps); err != nil {
		t.Fatalf("create kit failed: %v", err)
	}
	kitDir := filepath.Join(authorRoot, "acme")
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"create", "skill", "audit", "--kit-dir", kitDir}, deps); err != nil {
		t.Fatalf("create skill failed: %v", err)
	}
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"validate", kitDir}, deps); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	archV1 := filepath.Join(base, "acme-0.1.0.tar.gz")
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"pack", kitDir, "--out", archV1, "--json"}, deps); err != nil {
		t.Fatalf("pack v1 failed: %v", err)
	}
	var packV1Env Envelope
	if err := json.Unmarshal(out.Bytes(), &packV1Env); err != nil || packV1Env.Pack == nil {
		t.Fatalf("unexpected pack envelope: %v (%s)", err, out.String())
	}

	// Bump kit to 0.2.0 and pack v0.2.0.
	manifestBytes, _ := os.ReadFile(filepath.Join(kitDir, "package.json"))
	bumped := strings.Replace(string(manifestBytes), `"version": "0.1.0"`, `"version": "0.2.0"`, 1)
	if err := os.WriteFile(filepath.Join(kitDir, "package.json"), []byte(bumped), 0644); err != nil {
		t.Fatal(err)
	}
	archV2 := filepath.Join(base, "acme-0.2.0.tar.gz")
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"pack", kitDir, "--out", archV2, "--json"}, deps); err != nil {
		t.Fatalf("pack v2 failed: %v", err)
	}
	var packV2Env Envelope
	_ = json.Unmarshal(out.Bytes(), &packV2Env)

	v1Bytes, _ := os.ReadFile(archV1)
	v2Bytes, _ := os.ReadFile(archV2)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			_ = json.NewEncoder(w).Encode(dist.RegistryIndex{
				SchemaVersion: 1,
				Packages: []dist.IndexEntry{
					{ID: "acme", Version: "0.1.0", URL: srv.URL + "/acme-0.1.0.tar.gz", ArchiveSHA256: packV1Env.Pack.ArchiveSHA256, PackageDigest: packV1Env.Pack.PackageDigest},
					{ID: "acme", Version: "0.2.0", URL: srv.URL + "/acme-0.2.0.tar.gz", ArchiveSHA256: packV2Env.Pack.ArchiveSHA256, PackageDigest: packV2Env.Pack.PackageDigest},
				},
			})
		case "/acme-0.1.0.tar.gz":
			_, _ = w.Write(v1Bytes)
		case "/acme-0.2.0.tar.gz":
			_, _ = w.Write(v2Bytes)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	deps.HTTPClient = srv.Client()

	// 2. Install pinned acme@0.1.0 from remote registry URL.
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"install", "acme@0.1.0", "--project", "--source", srv.URL + "/index.json", "--yes", "--json"}, deps); err != nil {
		t.Fatalf("remote install failed: %v (%s)", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(proj, ".agents", "skills", "dtkit-project-audit", "SKILL.md")); err != nil {
		t.Fatalf("custom kit skill missing after remote install: %v", err)
	}

	// 3. Status works on custom remote-installed kit without touching network.
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"status", "--project", "--json"}, deps); err != nil {
		t.Fatalf("status on custom kit failed: %v", err)
	}

	// 4. Remote update resolves 0.2.0 from index.
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"update", "acme", "--project", "--source", srv.URL + "/index.json", "--yes", "--json"}, deps); err != nil {
		t.Fatalf("remote update failed: %v (%s)", err, out.String())
	}

	// 5. Remove custom kit, then reinstall 0.2.0 strictly offline using pinned SHA-256 from cache!
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"remove", "acme", "--project", "--yes"}, deps); err != nil {
		t.Fatalf("remove custom kit failed: %v", err)
	}
	srv.Close()
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"install", "acme@0.2.0", "--project", "--offline", "--sha256", packV2Env.Pack.ArchiveSHA256, "--yes", "--json"}, deps); err != nil {
		t.Fatalf("offline cached reinstall failed: %v (%s)", err, out.String())
	}

	// Verify missing cache entry in offline mode fails with ExitConflict.
	out.Reset()
	err = RunWithDeps(context.Background(), []string{"install", "acme@0.2.0", "--project", "--offline", "--sha256", strings.Repeat("b", 64), "--yes"}, deps)
	if apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict for offline cache miss, got %v", err)
	}
}
