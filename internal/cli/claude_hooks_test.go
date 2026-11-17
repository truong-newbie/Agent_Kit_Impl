package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestCLIClaudeHooksEnableDisableRemoveAndGlobalLifecycle(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(base, "repo")
	home := filepath.Join(base, "home")
	claudeCfg := filepath.Join(base, "claude-cfg")
	exe := filepath.Join(base, "bin", "dtkit")
	for _, d := range []string{repo, home, claudeCfg, filepath.Dir(exe)} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(exe, []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}

	settingsPath := filepath.Join(repo, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatal(err)
	}
	userSettings := "{\n  \"permissions\": {\"allow\": [\"Read\"]},\n  \"mcpServers\": {\"srv\": {\"command\": \"node\"}}\n}\n"
	if err := os.WriteFile(settingsPath, []byte(userSettings), 0644); err != nil {
		t.Fatal(err)
	}

	makeDeps := func() (Deps, *bytes.Buffer, *bytes.Buffer) {
		var out, errBuf bytes.Buffer
		return Deps{
			Stdin:         &bytes.Buffer{},
			Stdout:        &out,
			Stderr:        &errBuf,
			IsInteractive: func() bool { return false },
			Executable:    func() (string, error) { return exe, nil },
			Now:           func() time.Time { return time.Date(2026, 11, 17, 10, 30, 0, 0, time.UTC) },
			Env: paths.Environment{
				Getwd:       func() (string, error) { return repo, nil },
				UserHomeDir: func() (string, error) { return home, nil },
				Getenv: func(k string) string {
					if k == "CLAUDE_CONFIG_DIR" {
						return claudeCfg
					}
					return ""
				},
			},
		}, &out, &errBuf
	}

	// 1. Install Claude kit in project, then enable both check-changed & guard-edit.
	deps, _, _ := makeDeps()
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--path", repo, "--runtime", "claude", "--yes", "--json"}, deps); err != nil {
		t.Fatalf("claude install failed: %v", err)
	}
	for _, hookID := range []string{"check-changed", "guard-edit"} {
		deps, out, _ := makeDeps()
		if err := RunWithDeps(context.Background(), []string{"hooks", "enable", hookID, "--project", "--path", repo, "--runtime", "claude", "--executable", exe, "--yes", "--json"}, deps); err != nil {
			t.Fatalf("enable %s failed: %v\n%s", hookID, err, out.String())
		}
	}

	// Verify .claude/settings.json preserved user settings and registered both hooks.
	rawSettings, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(rawSettings), `"permissions"`) || !strings.Contains(string(rawSettings), `"mcpServers"`) || !strings.Contains(string(rawSettings), "PostToolUse") || !strings.Contains(string(rawSettings), "PreToolUse") {
		t.Fatalf("unexpected merged .claude/settings.json:\n%s", string(rawSettings))
	}
	deps, out, _ := makeDeps()
	if err := RunWithDeps(context.Background(), []string{"status", "--project", "--path", repo, "--runtime", "claude", "--json"}, deps); err != nil {
		t.Fatalf("claude status with hooks failed: %v\n%s", err, out.String())
	}
	var fullSt Envelope
	if err := json.Unmarshal(out.Bytes(), &fullSt); err != nil || len(fullSt.Installations) != 1 || len(fullSt.Installations[0].Hooks) != 2 {
		t.Fatalf("expected status to include 2 Claude hooks, got %+v", fullSt.Installations)
	}

	// 2. Disable guard-edit, verify check-changed stays intact.
	deps, out, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"hooks", "disable", "guard-edit", "--project", "--path", repo, "--runtime", "claude", "--yes", "--json"}, deps); err != nil {
		t.Fatalf("disable guard-edit failed: %v\n%s", err, out.String())
	}
	deps, out, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"hooks", "status", "--project", "--path", repo, "--runtime", "claude", "--json"}, deps); err != nil {
		t.Fatalf("hooks status failed: %v\n%s", err, out.String())
	}
	var stEnv Envelope
	if err := json.Unmarshal(out.Bytes(), &stEnv); err != nil || len(stEnv.Hooks) != 1 || stEnv.Hooks[0].ID != "check-changed" {
		t.Fatalf("unexpected Claude hook status: %+v err=%v", stEnv.Hooks, err)
	}

	// 3. Remove Claude kit cleanly unmerges check-changed and leaves user settings intact.
	deps, _, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"remove", "engineer", "--project", "--path", repo, "--runtime", "claude", "--yes"}, deps); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	afterRemove, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(afterRemove), `"mcpServers"`) || strings.Contains(string(afterRemove), "check-changed") {
		t.Fatalf("user settings altered or hook left behind:\n%s", string(afterRemove))
	}

	// 4. Global $CLAUDE_CONFIG_DIR/settings.json install + enable + disable.
	deps, _, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--global", "--runtime", "claude", "--yes"}, deps); err != nil {
		t.Fatalf("claude global install failed: %v", err)
	}
	deps, _, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"hooks", "enable", "check-changed", "--global", "--runtime", "claude", "--executable", exe, "--yes"}, deps); err != nil {
		t.Fatalf("claude global enable hook failed: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(claudeCfg, "settings.json")); statErr != nil {
		t.Fatalf("expected $CLAUDE_CONFIG_DIR/settings.json: %v", statErr)
	}
}
