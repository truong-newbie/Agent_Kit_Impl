package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func selectionTestEnv(base, cwd, home, claudeHome string) paths.Environment {
	return paths.Environment{
		Getwd:       func() (string, error) { return cwd, nil },
		UserHomeDir: func() (string, error) { return home, nil },
		Getenv: func(k string) string {
			switch k {
			case "HOME", "USERPROFILE":
				return home
			case "CLAUDE_CONFIG_DIR":
				return claudeHome
			case "CODEX_HOME":
				return filepath.Join(home, ".codex")
			case "XDG_CONFIG_HOME":
				return filepath.Join(base, "xdg-cfg")
			case "XDG_DATA_HOME":
				return filepath.Join(base, "xdg-data")
			case "XDG_STATE_HOME":
				return filepath.Join(base, "xdg-state")
			case "APPDATA":
				return filepath.Join(base, "appdata")
			case "LOCALAPPDATA":
				return filepath.Join(base, "localappdata")
			}
			return ""
		},
	}
}

func runSelectionCLI(t *testing.T, env paths.Environment, args ...string) (Envelope, string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	deps := Deps{
		Stdout:        &stdout,
		Stderr:        &stderr,
		Env:           env,
		IsInteractive: func() bool { return false },
		LookPath:      func(file string) (string, error) { return "/usr/bin/" + file, nil },
		Now:           func() time.Time { return time.Date(2026, 11, 17, 11, 30, 0, 0, time.UTC) },
	}

	err := RunWithDeps(context.Background(), args, deps)
	var envelope Envelope
	for _, a := range args {
		if a == "--json" && stdout.Len() > 0 {
			if jErr := json.Unmarshal(stdout.Bytes(), &envelope); jErr != nil {
				t.Fatalf("invalid JSON output for %v: %v\nraw:\n%s", args, jErr, stdout.String())
			}
			break
		}
	}
	return envelope, stdout.String(), stderr.String(), err
}

func TestCLISelectionValidationErrors(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "repo")
	home := filepath.Join(base, "home")
	claudeHome := filepath.Join(base, "claude-home")
	for _, d := range []string{proj, home, claudeHome} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	env := selectionTestEnv(base, proj, home, claudeHome)

	cases := []struct {
		name string
		args []string
		code string
	}{
		{
			name: "conflicting preset and components",
			args: []string{"install", "engineer", "--project", "--preset", "minimal", "--components", "skill/ask", "--dry-run", "--json"},
			code: "conflicting_selection",
		},
		{
			name: "unknown preset",
			args: []string{"install", "engineer", "--project", "--preset", "nonexistent", "--dry-run", "--json"},
			code: "unknown_preset",
		},
		{
			name: "unknown component",
			args: []string{"install", "engineer", "--project", "--components", "skill/missing", "--dry-run", "--json"},
			code: "unknown_component",
		},
		{
			name: "empty components value",
			args: []string{"install", "engineer", "--project", "--components=", "--dry-run", "--json"},
			code: "empty_components",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			envelope, _, _, runErr := runSelectionCLI(t, env, tc.args...)
			if runErr == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if apperr.ExitCodeOf(runErr) != apperr.ExitUsage {
				t.Fatalf("exit code = %d, want ExitUsage (%d): %v", apperr.ExitCodeOf(runErr), apperr.ExitUsage, runErr)
			}
			var jsonErr *JSONReportedError
			if !errors.As(runErr, &jsonErr) {
				t.Fatalf("expected JSONReportedError, got %T", runErr)
			}
			if envelope.Ok || len(envelope.Errors) == 0 || envelope.Errors[0].Code != tc.code {
				t.Fatalf("expected error code %q in envelope, got %+v", tc.code, envelope.Errors)
			}
		})
	}
}

func TestCLISelectionInstallStatusUpdatePreserveAndSwitch(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "repo")
	home := filepath.Join(base, "home")
	claudeHome := filepath.Join(base, "claude-home")
	vNextDir := filepath.Join(base, "engineer-vnext")
	for _, d := range []string{proj, home, claudeHome, vNextDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	env := selectionTestEnv(base, proj, home, claudeHome)

	// Install minimal preset on Claude.
	instEnv, _, _, err := runSelectionCLI(t, env, "install", "engineer", "--project", "--runtime", "claude", "--preset", "minimal", "--yes", "--json")
	if err != nil {
		t.Fatalf("install --preset minimal failed: %v", err)
	}
	if !instEnv.Ok || instEnv.Selection == nil || instEnv.Selection.Preset != "minimal" {
		t.Fatalf("unexpected install selection envelope: %+v", instEnv.Selection)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-plan", "SKILL.md")); err != nil {
		t.Fatalf("expected dtkit-claude-project-plan skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-review", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("did not expect dtkit-claude-project-review skill under minimal preset")
	}

	// Status exposes recorded selection and passes cleanly.
	statusEnv, _, _, err := runSelectionCLI(t, env, "status", "--project", "--runtime", "claude", "--json")
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	if statusEnv.Selection == nil || statusEnv.Selection.Preset != "minimal" {
		t.Fatalf("expected status selection preset minimal, got %+v", statusEnv.Selection)
	}

	// Re-installing with a different preset must fail cleanly with ExitConflict.
	_, _, _, conflictErr := runSelectionCLI(t, env, "install", "engineer", "--project", "--runtime", "claude", "--preset", "review-only", "--yes")
	if conflictErr == nil || apperr.ExitCodeOf(conflictErr) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict when re-installing different preset, got %v", conflictErr)
	}

	// Materialize bundled engineer kit at version 0.5.0 in vNextDir and verify dtkit update preserves preset minimal.
	_, engFS, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.WalkDir(engFS, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || rel == "." {
			return walkErr
		}
		target := filepath.Join(vNextDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, rErr := fs.ReadFile(engFS, rel)
		if rErr != nil {
			return rErr
		}
		if rel == "package.json" {
			data = []byte(strings.Replace(string(data), `"version": "0.5.0"`, `"version": "0.6.0"`, 1))
		}
		return os.WriteFile(target, data, 0644)
	}); err != nil {
		t.Fatal(err)
	}

	updPreserveEnv, _, _, err := runSelectionCLI(t, env, "update", "engineer", "--project", "--runtime", "claude", "--from", vNextDir, "--yes", "--json")
	if err != nil {
		t.Fatalf("update preserving minimal preset failed: %v", err)
	}
	if updPreserveEnv.Selection == nil || updPreserveEnv.Selection.Preset != "minimal" || updPreserveEnv.Kit.Version != "0.6.0" {
		t.Fatalf("expected 0.6.0 upgrade to preserve minimal preset, got kit=%+v selection=%+v", updPreserveEnv.Kit, updPreserveEnv.Selection)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-review", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("expected minimal preset preservation to omit dtkit-claude-project-review")
	}

	// Switch explicitly to --components skill/review (should remove dtkit-claude-project-plan and install dtkit-claude-project-review).
	switchEnv, _, _, err := runSelectionCLI(t, env, "update", "engineer", "--project", "--runtime", "claude", "--from", vNextDir, "--components", "skill/review", "--yes", "--json")
	if err != nil {
		t.Fatalf("update --components skill/review failed: %v", err)
	}
	wantResolved := []string{"rule/engineering", "skill/review", "template/review-report"}
	if switchEnv.Selection == nil || switchEnv.Selection.Preset != "" ||
		!reflect.DeepEqual(switchEnv.Selection.RequestedComponents, []string{"skill/review"}) ||
		!reflect.DeepEqual(switchEnv.Selection.ResolvedComponents, wantResolved) {
		t.Fatalf("unexpected selection after --components switch: %+v", switchEnv.Selection)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-review", "SKILL.md")); err != nil {
		t.Fatalf("expected dtkit-claude-project-review installed after component switch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-plan", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("expected dtkit-claude-project-plan removed after component switch")
	}
}
