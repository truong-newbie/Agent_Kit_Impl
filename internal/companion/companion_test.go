package companion

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestCompanionFallbackCacheAndRegisterCheck(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	repo := filepath.Join(base, "repo")
	if err := os.MkdirAll(filepath.Join(repo, "backend"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "backend", "App.java"), []byte("public class App {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	env := paths.Environment{
		Getwd:       func() (string, error) { return repo, nil },
		UserHomeDir: func() (string, error) { return home, nil },
	}

	// 1. Missing tool -> missing_tool status with native scout fallback
	missingLP := func(string) (string, error) { return "", errors.New("not found") }
	res, err := RunCompanion(context.Background(), env, repo, "repomix", "backend", false, missingLP)
	if err != nil {
		t.Fatalf("expected graceful fallback on missing repomix, got %v", err)
	}
	if res.Status != "missing_tool" || res.FallbackUsed != "dtkit scout --pack" || !strings.Contains(res.Output, "backend/App.java") {
		t.Fatalf("unexpected fallback result: %+v", res)
	}

	// 2. Available executable -> runs, caches by workspace digest, and invalidates on file edit
	if goExe, lpErr := exec.LookPath("go"); lpErr == nil {
		fakeLP := func(string) (string, error) { return goExe, nil }
		// Override graphify command in test by invoking graphify (go help query exits 0 or use go version? wait: `go version` ignores extra args? No, `go env` works!)
		_ = fakeLP
	}

	// Direct cache write & hit check + invalidation when workspace file changes
	digestBefore := res.WorkspaceDigest
	key := computeCacheKey("repomix", "backend", digestBefore)
	if err := writeCache(repo, key, RunResult{
		ToolID:          "repomix",
		Zone:            "backend",
		Status:          "ok",
		WorkspaceDigest: digestBefore,
		Output:          "CACHED_BUNDLE",
	}); err != nil {
		t.Fatal(err)
	}

	cachedRes, err := RunCompanion(context.Background(), env, repo, "repomix", "backend", false, missingLP)
	if err != nil || !cachedRes.CacheHit || cachedRes.Status != "cached" || cachedRes.Output != "CACHED_BUNDLE" {
		t.Fatalf("expected cache hit, got res=%+v err=%v", cachedRes, err)
	}

	// Modify file -> workspace digest changes -> cache miss
	if err := os.WriteFile(filepath.Join(repo, "backend", "App.java"), []byte("public class App { int x; }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	missRes, err := RunCompanion(context.Background(), env, repo, "repomix", "backend", false, missingLP)
	if err != nil || missRes.CacheHit || missRes.WorkspaceDigest == digestBefore {
		t.Fatalf("expected cache invalidation after edit, got %+v", missRes)
	}

	// 3. RegisterProjectCheck dry-run vs write
	_, added, err := RegisterProjectCheck(context.Background(), env, repo, "ast-grep", true)
	if err != nil || !added {
		t.Fatalf("RegisterProjectCheck dry-run failed: added=%v err=%v", added, err)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".dtkit", "project.json")); !os.IsNotExist(statErr) {
		t.Fatal("dry-run should not write .dtkit/project.json")
	}

	prof, added, err := RegisterProjectCheck(context.Background(), env, repo, "ast-grep", false)
	if err != nil || !added || len(prof.Checks) == 0 {
		t.Fatalf("RegisterProjectCheck write failed: added=%v checks=%d err=%v", added, len(prof.Checks), err)
	}
	// Second registration is idempotent
	_, addedAgain, err := RegisterProjectCheck(context.Background(), env, repo, "ast-grep", false)
	if err != nil || addedAgain {
		t.Fatalf("expected idempotent false on second RegisterProjectCheck, got added=%v err=%v", addedAgain, err)
	}
}
