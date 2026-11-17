package cli

import (
	"bytes"
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
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
)

func TestCLILockGenerateSignVerifySyncFreshCloneAndRevocation(t *testing.T) {
	// Author custom kit and serve archive + index over local TLS.
	kitBuildParent := t.TempDir()
	if err := RunWithDeps(context.Background(), []string{"create", "kit", "acme", "--dir", kitBuildParent}, DefaultDeps(nil, &bytes.Buffer{}, &bytes.Buffer{})); err != nil {
		t.Fatalf("create kit failed: %v", err)
	}
	acmeDir := filepath.Join(kitBuildParent, "acme")
	archOut := filepath.Join(t.TempDir(), "acme-0.1.0.tar.gz")
	var packOut bytes.Buffer
	if err := RunWithDeps(context.Background(), []string{"pack", acmeDir, "--out", archOut, "--json"}, DefaultDeps(nil, &packOut, &bytes.Buffer{})); err != nil {
		t.Fatalf("pack failed: %v", err)
	}
	var packEnv Envelope
	if err := json.Unmarshal(packOut.Bytes(), &packEnv); err != nil {
		t.Fatal(err)
	}
	archBytes, err := os.ReadFile(archOut)
	if err != nil {
		t.Fatal(err)
	}

	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			_ = json.NewEncoder(w).Encode(dist.RegistryIndex{
				SchemaVersion: 1,
				Packages: []dist.IndexEntry{
					{ID: "acme", Version: "0.1.0", URL: srv.URL + "/acme-0.1.0.tar.gz", ArchiveSHA256: packEnv.Pack.ArchiveSHA256, PackageDigest: packEnv.Pack.PackageDigest},
				},
			})
		case "/acme-0.1.0.tar.gz":
			_, _ = w.Write(archBytes)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// --- Machine A: install bundled engineer (minimal) + remote acme, keygen, lock generate, sign, verify ---
	homeA := filepath.Join(t.TempDir(), "alice-home")
	cacheA := filepath.Join(homeA, "cache")
	projA := filepath.Join(homeA, "work", "repoA")
	for _, d := range []string{homeA, cacheA, projA} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	runA := func(args ...string) (Envelope, string, error) {
		var out, errBuf bytes.Buffer
		deps := DefaultDeps(nil, &out, &errBuf)
		deps.Env = paths.Environment{
			Getwd:        func() (string, error) { return projA, nil },
			UserHomeDir:  func() (string, error) { return homeA, nil },
			UserCacheDir: func() (string, error) { return cacheA, nil },
			Getenv:       func(k string) string { return "" },
		}
		deps.HTTPClient = srv.Client()
		err := RunWithDeps(context.Background(), args, deps)
		var resp Envelope
		if bytes.Contains(out.Bytes(), []byte(`"schemaVersion"`)) {
			_ = json.Unmarshal(out.Bytes(), &resp)
		}
		return resp, out.String(), err
	}

	if _, raw, err := runA("install", "engineer", "--preset", "minimal", "--project", "--yes", "--json"); err != nil {
		t.Fatalf("Machine A install engineer failed: %v (%s)", err, raw)
	}
	if _, raw, err := runA("install", "acme@0.1.0", "--source", srv.URL+"/index.json", "--project", "--yes", "--json"); err != nil {
		t.Fatalf("Machine A install acme failed: %v (%s)", err, raw)
	}

	keyPrefix := filepath.Join(t.TempDir(), "team-signing")
	if _, raw, err := runA("lock", "keygen", "--key-id", "team-k1", "--publisher", "team-sec", "--out-prefix", keyPrefix, "--json"); err != nil {
		t.Fatalf("keygen failed: %v (%s)", err, raw)
	}
	trustBytes, err := os.ReadFile(keyPrefix + ".trust.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projA, ".dtkit"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projA, ".dtkit", "trust.json"), trustBytes, 0644); err != nil {
		t.Fatal(err)
	}

	if _, raw, err := runA("lock", "generate", "--source-ref", "acme="+srv.URL+"/index.json", "--sign-key", keyPrefix+".key.b64", "--key-id", "team-k1", "--yes", "--json"); err != nil {
		t.Fatalf("lock generate failed: %v (%s)", err, raw)
	}

	lockBytes, err := os.ReadFile(filepath.Join(projA, "dtkit.lock"))
	if err != nil {
		t.Fatal(err)
	}
	// Portability check: dtkit.lock must never embed Machine A paths or credentials.
	if strings.Contains(string(lockBytes), "alice-home") || strings.Contains(string(lockBytes), "repoA") {
		t.Fatalf("dtkit.lock leaked local machine paths:\n%s", string(lockBytes))
	}

	if _, raw, err := runA("lock", "verify", "--require-signature", "--json"); err != nil {
		t.Fatalf("lock verify failed: %v (%s)", err, raw)
	}

	// Shut down remote server so Machine B MUST rely strictly on verified cache & dtkit.lock offline!
	srv.Close()

	// --- Machine B: fresh clean clone, different home/path, populated verified cache, offline sync ---
	homeB := filepath.Join(t.TempDir(), "bob-home")
	cacheB := filepath.Join(homeB, "cache")
	projB := filepath.Join(homeB, "dev", "repoB")
	for _, d := range []string{homeB, cacheB, projB, filepath.Join(projB, ".dtkit")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projB, "dtkit.lock"), lockBytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projB, ".dtkit", "trust.json"), trustBytes, 0644); err != nil {
		t.Fatal(err)
	}
	// Seed Machine B verified cache with archive bytes (mirrors pre-populated team/CI cache).
	cacheObjB, err := dist.OpenCache(paths.Environment{UserCacheDir: func() (string, error) { return cacheB, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := cacheObjB.PutArchive(archBytes, &dist.CacheMetadata{SchemaVersion: 1, ArchiveSHA256: packEnv.Pack.ArchiveSHA256, PackageDigest: packEnv.Pack.PackageDigest}); err != nil {
		t.Fatal(err)
	}

	runB := func(args ...string) (Envelope, string, error) {
		var out, errBuf bytes.Buffer
		deps := DefaultDeps(nil, &out, &errBuf)
		deps.Env = paths.Environment{
			Getwd:        func() (string, error) { return projB, nil },
			UserHomeDir:  func() (string, error) { return homeB, nil },
			UserCacheDir: func() (string, error) { return cacheB, nil },
			Getenv:       func(k string) string { return "" },
		}
		err := RunWithDeps(context.Background(), args, deps)
		var resp Envelope
		if bytes.Contains(out.Bytes(), []byte(`"schemaVersion"`)) {
			_ = json.Unmarshal(out.Bytes(), &resp)
		}
		return resp, out.String(), err
	}

	// Dry-run sync writes nothing.
	prevEnv, prevRaw, err := runB("sync", "--offline", "--require-signature", "--dry-run", "--diff", "--json")
	if err != nil || len(prevEnv.SyncResults) != 2 {
		t.Fatalf("Machine B sync dry-run failed: err=%v (%s)", err, prevRaw)
	}
	if _, err := os.Stat(filepath.Join(projB, ".dtkit", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("sync --dry-run must not write state.json")
	}

	// Real offline sync installs byte-identical files as Machine A.
	syncEnv, syncRaw, err := runB("sync", "--offline", "--require-signature", "--yes", "--json")
	if err != nil || len(syncEnv.SyncResults) != 2 {
		t.Fatalf("Machine B sync apply failed: err=%v (%s)", err, syncRaw)
	}
	for _, rel := range []string{
		".agents/skills/dtkit-project-ask/SKILL.md",
		".agents/skills/dtkit-project-acme-example/SKILL.md",
	} {
		bA, errA := os.ReadFile(filepath.Join(projA, filepath.FromSlash(rel)))
		bB, errB := os.ReadFile(filepath.Join(projB, filepath.FromSlash(rel)))
		if errA != nil || errB != nil || !bytes.Equal(bA, bB) {
			t.Fatalf("fresh clone %s byte mismatch between Machine A and Machine B (errA=%v, errB=%v)", rel, errA, errB)
		}
	}
	agentsA, err := os.ReadFile(filepath.Join(projA, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	agentsB, err := os.ReadFile(filepath.Join(projB, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, kitID := range []string{"acme", "engineer"} {
		blkA, okA, eA := planner.ExtractBlockForRuntime("codex", kitID, agentsA, "project")
		blkB, okB, eB := planner.ExtractBlockForRuntime("codex", kitID, agentsB, "project")
		if eA != nil || eB != nil || !okA || !okB || !bytes.Equal(blkA, blkB) {
			t.Fatalf("fresh clone AGENTS.md %s block mismatch between Machine A and Machine B (okA=%v okB=%v eA=%v eB=%v)", kitID, okA, okB, eA, eB)
		}
	}

	// Second sync is cleanly idempotent (noop).
	noopEnv, noopRaw, err := runB("sync", "--offline", "--require-signature", "--yes", "--json")
	if err != nil {
		t.Fatalf("idempotent sync failed: %v (%s)", err, noopRaw)
	}
	for _, r := range noopEnv.SyncResults {
		if r.Action != "noop" {
			t.Fatalf("expected noop on second sync, got %+v", r)
		}
	}

	// --- Machine C: revoked key fails BEFORE any mutation ---
	projC := filepath.Join(t.TempDir(), "repoC")
	if err := os.MkdirAll(filepath.Join(projC, ".dtkit"), 0755); err != nil {
		t.Fatal(err)
	}
	revokedTrust := strings.Replace(string(trustBytes), `"status": "active"`, `"status": "revoked"`, 1)
	if err := os.WriteFile(filepath.Join(projC, "dtkit.lock"), lockBytes, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projC, ".dtkit", "trust.json"), []byte(revokedTrust), 0644); err != nil {
		t.Fatal(err)
	}
	var outC bytes.Buffer
	depsC := DefaultDeps(nil, &outC, &bytes.Buffer{})
	depsC.Env = paths.Environment{
		Getwd:        func() (string, error) { return projC, nil },
		UserHomeDir:  func() (string, error) { return homeB, nil },
		UserCacheDir: func() (string, error) { return cacheB, nil },
	}
	syncErrC := RunWithDeps(context.Background(), []string{"sync", "--offline", "--require-signature", "--yes", "--json"}, depsC)
	if apperr.ExitCodeOf(syncErrC) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict on revoked key sync, got %v (%s)", syncErrC, outC.String())
	}
	if _, err := os.Stat(filepath.Join(projC, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("revoked key verification must fail BEFORE writing AGENTS.md")
	}
}
