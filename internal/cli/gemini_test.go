package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

func TestGeminiCLILifecycleAndCoexistence(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(base, "codex-home")
	geminiBase := filepath.Join(base, "gemini-cli-home")
	proj := filepath.Join(base, "repo")
	for _, d := range []string{home, codexHome, geminiBase, filepath.Join(proj, ".gemini")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	makeDeps := func(hasGemini bool) (Deps, *strings.Builder) {
		deps, out, _ := testDeps("", false, home, codexHome, proj, true)
		var outBuf strings.Builder
		deps.Stdout = &outBuf
		_ = out
		prevGetenv := deps.Env.Getenv
		deps.Env.Getenv = func(k string) string {
			if k == "GEMINI_CLI_HOME" {
				return geminiBase
			}
			return prevGetenv(k)
		}
		deps.LookPath = func(file string) (string, error) {
			if file == "codex" {
				return "/usr/local/bin/codex", nil
			}
			if file == "gemini" && hasGemini {
				return "/usr/local/bin/gemini", nil
			}
			return "", os.ErrNotExist
		}
		return deps, &outBuf
	}

	// 1. doctor --runtime gemini returns exit 4 when binary is missing.
	depsMissing, outMissing := makeDeps(false)
	err := RunWithDeps(context.Background(), []string{"doctor", "--runtime", "gemini", "--project", "--json"}, depsMissing)
	if apperr.ExitCodeOf(err) != apperr.ExitRuntime {
		t.Fatalf("expected ExitRuntime when gemini missing, got %d (%v)\n%s", apperr.ExitCodeOf(err), err, outMissing.String())
	}

	// 2. Write overriding .gemini/settings.json and verify doctor --runtime gemini warns cleanly & passes when binary present.
	if err := os.WriteFile(filepath.Join(proj, ".gemini", "settings.json"), []byte(`{"context":{"fileName":"CUSTOM.md"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	depsReady, outReady := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"doctor", "--runtime", "gemini", "--project", "--json"}, depsReady); err != nil {
		t.Fatalf("doctor --runtime gemini failed: %v\n%s", err, outReady.String())
	}
	var docEnv Envelope
	if err := json.Unmarshal([]byte(outReady.String()), &docEnv); err != nil || docEnv.Runtime != "gemini" {
		t.Fatalf("unexpected doctor envelope: %+v err=%v", docEnv, err)
	}
	sawOverrideWarn := false
	for _, c := range docEnv.Checks {
		if c.Name == "context_settings" && c.Status == "warn" {
			sawOverrideWarn = true
		}
	}
	if !sawOverrideWarn {
		t.Fatalf("expected context_settings warning in doctor checks: %+v", docEnv.Checks)
	}

	// 3. Validate bundled engineer kit explicitly for gemini.
	depsVal, outVal := makeDeps(true)
	repoEng := filepath.Join("..", "catalog", "engineer")
	if err := RunWithDeps(context.Background(), []string{"validate", repoEng, "--runtime", "gemini", "--json"}, depsVal); err != nil {
		t.Fatalf("validate --runtime gemini failed: %v\n%s", err, outVal.String())
	}

	// 4. Install both Codex and Gemini in the same project + global scope cleanly.
	depsInstallCodex, _ := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--runtime", "codex", "--yes"}, depsInstallCodex); err != nil {
		t.Fatalf("codex install failed: %v", err)
	}
	depsInstallGemini, _ := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--runtime", "gemini", "--yes"}, depsInstallGemini); err != nil {
		t.Fatalf("gemini install failed: %v", err)
	}
	depsInstallGlobalGem, _ := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--global", "--runtime", "gemini", "--yes"}, depsInstallGlobalGem); err != nil {
		t.Fatalf("gemini global install failed: %v", err)
	}

	// Verify both GEMINI.md files and global skill exist.
	if _, err := os.Stat(filepath.Join(proj, "GEMINI.md")); err != nil {
		t.Fatalf("missing project GEMINI.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(geminiBase, ".gemini", "GEMINI.md")); err != nil {
		t.Fatalf("missing global GEMINI.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(geminiBase, ".gemini", "skills", "dtkit-gemini-global-plan", "SKILL.md")); err != nil {
		t.Fatalf("missing global gemini skill: %v", err)
	}

	// 5. Status without --runtime in same project reports both Codex and Gemini.
	depsStatus, outStatus := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"status", "--project", "--json"}, depsStatus); err != nil {
		t.Fatalf("status --project failed: %v\n%s", err, outStatus.String())
	}
	var statusEnv Envelope
	if err := json.Unmarshal([]byte(outStatus.String()), &statusEnv); err != nil {
		t.Fatal(err)
	}
	if len(statusEnv.Installations) != 2 {
		t.Fatalf("expected 2 installations (codex+gemini), got %d: %+v", len(statusEnv.Installations), statusEnv.Installations)
	}

	// 6. Hooks command for Gemini rejects cleanly (explicit unsupported capability).
	depsHooks, _ := makeDeps(true)
	err = RunWithDeps(context.Background(), []string{"hooks", "enable", "session-context", "--project", "--runtime", "gemini", "--yes"}, depsHooks)
	if apperr.ExitCodeOf(err) != apperr.ExitUsage || !strings.Contains(err.Error(), "does not manage hooks for Gemini CLI") {
		t.Fatalf("expected clean unsupported hooks usage error for gemini, got %v", err)
	}

	// 7. Remove Gemini installation leaves Codex project install untouched.
	depsRemove, _ := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"remove", "engineer", "--project", "--runtime", "gemini", "--yes"}, depsRemove); err != nil {
		t.Fatalf("gemini remove failed: %v", err)
	}
	depsStatusAfter, outAfter := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"status", "--project", "--json"}, depsStatusAfter); err != nil {
		t.Fatalf("status after remove failed: %v\n%s", err, outAfter.String())
	}
	var afterEnv Envelope
	if err := json.Unmarshal([]byte(outAfter.String()), &afterEnv); err != nil {
		t.Fatal(err)
	}
	if len(afterEnv.Installations) != 1 || afterEnv.Installations[0].Runtime != "codex" || !afterEnv.Installations[0].Installed {
		t.Fatalf("expected only healthy codex left, got %+v", afterEnv.Installations)
	}
}
