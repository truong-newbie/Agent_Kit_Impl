package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestMultiRuntimeQualityHookLifecycles(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	geminiHome := filepath.Join(home, ".gemini-custom")
	openCodeHome := filepath.Join(home, ".opencode-custom")
	proj := filepath.Join(t.TempDir(), "repo")
	exeDir := t.TempDir()
	exeName := "dtkit-stable"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	exePath := filepath.Join(exeDir, exeName)
	for _, d := range []string{home, geminiHome, openCodeHome, proj, filepath.Join(proj, ".gemini"), filepath.Join(proj, ".cursor"), filepath.Join(proj, ".opencode", "plugins")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(exePath, []byte("stub"), 0755); err != nil {
		t.Fatal(err)
	}

	// Pre-seed user configs across all 3 runtimes that MUST stay intact through enable/update/disable/remove.
	gemSettingsPath := filepath.Join(proj, ".gemini", "settings.json")
	if err := os.WriteFile(gemSettingsPath, []byte("{\n  \"context\": {\"fileName\": \"GEMINI.md\"}\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	curHooksPath := filepath.Join(proj, ".cursor", "hooks.json")
	if err := os.WriteFile(curHooksPath, []byte("{\n  \"version\": 1,\n  \"hooks\": {\"afterFileEdit\": [{\"command\": \"./user-lint.sh\"}]}\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ocUserPlugin := filepath.Join(proj, ".opencode", "plugins", "user-custom.ts")
	if err := os.WriteFile(ocUserPlugin, []byte("export const UserPlugin = async () => ({});\n"), 0644); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) (Envelope, string, error) {
		var out, errBuf bytes.Buffer
		deps := DefaultDeps(nil, &out, &errBuf)
		deps.Env = paths.Environment{
			Getwd:       func() (string, error) { return proj, nil },
			UserHomeDir: func() (string, error) { return home, nil },
			Getenv: func(k string) string {
				switch k {
				case "HOME", "USERPROFILE":
					return home
				case "GEMINI_CLI_HOME":
					return geminiHome
				case "OPENCODE_CONFIG_DIR":
					return openCodeHome
				default:
					return ""
				}
			},
		}
		deps.LookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
		deps.Executable = func() (string, error) { return exePath, nil }
		err := RunWithDeps(context.Background(), args, deps)
		var resp Envelope
		if bytes.Contains(out.Bytes(), []byte(`"schemaVersion"`)) {
			_ = json.Unmarshal(out.Bytes(), &resp)
		}
		return resp, out.String(), err
	}

	for _, rt := range []string{"gemini", "cursor", "opencode"} {
		if _, raw, err := run("install", "engineer", "--runtime", rt, "--preset", "minimal", "--project", "--yes", "--json"); err != nil {
			t.Fatalf("install %s failed: %v (%s)", rt, err, raw)
		}
		if _, raw, err := run("hooks", "enable", "check-changed", "--runtime", rt, "--project", "--executable", exePath, "--yes", "--json"); err != nil {
			t.Fatalf("hooks enable %s failed: %v (%s)", rt, err, raw)
		}

		// Project vs global scope dedupe blocks duplicate hook enable on the same runtime.
		if _, raw, err := run("install", "engineer", "--runtime", rt, "--preset", "minimal", "--global", "--yes", "--json"); err != nil {
			t.Fatalf("global install %s failed: %v (%s)", rt, err, raw)
		}
		_, _, dedupeErr := run("hooks", "enable", "check-changed", "--runtime", rt, "--global", "--executable", exePath, "--yes")
		if apperr.ExitCodeOf(dedupeErr) != apperr.ExitConflict {
			t.Fatalf("%s expected hook_scope_conflict ExitConflict, got %v", rt, dedupeErr)
		}
		if _, raw, err := run("hooks", "status", "--runtime", rt, "--project", "--json"); err != nil {
			t.Fatalf("%s hooks status failed: %v (%s)", rt, err, raw)
		}
	}

	// Verify OpenCode standalone plugin exists alongside user-custom.ts.
	ocPluginPath := filepath.Join(proj, ".opencode", "plugins", "dtkit-quality.js")
	if _, err := os.Stat(ocPluginPath); err != nil {
		t.Fatalf("missing OpenCode plugin: %v", err)
	}

	// Remove all 3 project installations and confirm hooks & plugins unmerge cleanly while preserving user configs.
	for _, rt := range []string{"gemini", "cursor", "opencode"} {
		if _, raw, err := run("remove", "engineer", "--runtime", rt, "--project", "--yes", "--json"); err != nil {
			t.Fatalf("remove %s failed: %v (%s)", rt, err, raw)
		}
	}

	gemAfter, _ := os.ReadFile(gemSettingsPath)
	if !strings.Contains(string(gemAfter), `"GEMINI.md"`) || strings.Contains(string(gemAfter), "AfterTool") {
		t.Fatalf("Gemini settings.json damaged after remove:\n%s", string(gemAfter))
	}
	curAfter, _ := os.ReadFile(curHooksPath)
	if !strings.Contains(string(curAfter), `./user-lint.sh`) || strings.Contains(string(curAfter), "dtkit") {
		t.Fatalf("Cursor hooks.json damaged after remove:\n%s", string(curAfter))
	}
	if _, err := os.Stat(ocPluginPath); !os.IsNotExist(err) {
		t.Fatalf("expected OpenCode dtkit-quality.js removed")
	}
	if _, err := os.Stat(ocUserPlugin); err != nil {
		t.Fatalf("expected user-custom.ts OpenCode plugin preserved: %v", err)
	}
}
