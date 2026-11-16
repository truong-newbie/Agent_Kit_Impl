package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

func TestCursorLifecycleFallbackAndFourWayCoexistence(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(base, "codex-home")
	geminiBase := filepath.Join(base, "gemini-cli-home")
	ocDir := filepath.Join(base, "opencode-cfg")
	proj := filepath.Join(base, "repo")
	for _, d := range []string{home, codexHome, geminiBase, ocDir, filepath.Join(proj, ".cursor", "rules")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	userRule := "---\ndescription: User personal Cursor rule\nalwaysApply: false\n---\nKeep tests green.\n"
	userRulePath := filepath.Join(proj, ".cursor", "rules", "personal.mdc")
	if err := os.WriteFile(userRulePath, []byte(userRule), 0644); err != nil {
		t.Fatal(err)
	}

	makeDeps := func(hasCursor bool) (Deps, *bytes.Buffer) {
		deps, _, _ := testDeps("", false, home, codexHome, proj, true)
		var outBuf bytes.Buffer
		deps.Stdout = &outBuf
		prevGetenv := deps.Env.Getenv
		deps.Env.Getenv = func(k string) string {
			switch k {
			case "GEMINI_CLI_HOME":
				return geminiBase
			case "OPENCODE_CONFIG_DIR":
				return ocDir
			default:
				return prevGetenv(k)
			}
		}
		deps.LookPath = func(file string) (string, error) {
			if file == "codex" || file == "gemini" || file == "opencode" {
				return "/usr/local/bin/" + file, nil
			}
			if file == "cursor" && hasCursor {
				return "/usr/local/bin/cursor", nil
			}
			return "", os.ErrNotExist
		}
		return deps, &outBuf
	}

	// 1. doctor --runtime cursor returns exit 4 when binary missing, and warns cleanly on global User Rules UI fallback when present.
	depsMiss, _ := makeDeps(false)
	err := RunWithDeps(context.Background(), []string{"doctor", "--runtime", "cursor", "--json"}, depsMiss)
	if apperr.ExitCodeOf(err) != apperr.ExitRuntime {
		t.Fatalf("expected ExitRuntime when cursor missing, got %v", err)
	}
	depsReady, outDoc := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"doctor", "--runtime", "cursor", "--json"}, depsReady); err != nil {
		t.Fatalf("doctor --runtime cursor failed: %v\n%s", err, outDoc.String())
	}
	var docEnv Envelope
	if err := json.Unmarshal(outDoc.Bytes(), &docEnv); err != nil {
		t.Fatal(err)
	}
	sawGlobalInstrNote := false
	for _, c := range docEnv.Checks {
		if c.Name == "instructions_capability" && c.Scope == "global" && c.Status == "warn" {
			sawGlobalInstrNote = true
		}
	}
	if !sawGlobalInstrNote {
		t.Fatalf("expected global Cursor User Rules fallback note in doctor: %+v", docEnv.Checks)
	}

	// 2. Install all 4 runtimes (codex, gemini, opencode, cursor) in the SAME project!
	for _, rt := range []string{"codex", "gemini", "opencode", "cursor"} {
		d, _ := makeDeps(true)
		if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--runtime", rt, "--yes"}, d); err != nil {
			t.Fatalf("install %s failed: %v", rt, err)
		}
	}
	ruleBytes, err := os.ReadFile(filepath.Join(proj, ".cursor", "rules", "dtkit-engineer.mdc"))
	if err != nil || !strings.Contains(string(ruleBytes), "alwaysApply: true") {
		t.Fatalf("missing or invalid cursor project rule: %q err=%v", string(ruleBytes), err)
	}

	// 3. Install Cursor globally and verify fallback warning about Cursor Settings UI User Rules.
	depsGlob, outGlob := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--global", "--runtime", "cursor", "--yes", "--json"}, depsGlob); err != nil {
		t.Fatalf("global cursor install failed: %v\n%s", err, outGlob.String())
	}
	var globEnv Envelope
	if err := json.Unmarshal(outGlob.Bytes(), &globEnv); err != nil {
		t.Fatal(err)
	}
	sawGlobWarn := false
	for _, w := range globEnv.Warnings {
		if strings.Contains(w, "Cursor Settings UI") {
			sawGlobWarn = true
		}
	}
	if !sawGlobWarn {
		t.Fatalf("expected global install capability warning for Cursor UI User Rules, got %+v", globEnv.Warnings)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "skills", "dtkit-cursor-global-plan", "SKILL.md")); err != nil {
		t.Fatalf("missing global cursor skill: %v", err)
	}

	// 4. Status across all 4 runtimes in the same repo reports 4 healthy installations.
	depsSt, outSt := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"status", "--project", "--json"}, depsSt); err != nil {
		t.Fatalf("status --project failed: %v\n%s", err, outSt.String())
	}
	var stEnv Envelope
	if err := json.Unmarshal(outSt.Bytes(), &stEnv); err != nil {
		t.Fatal(err)
	}
	if len(stEnv.Installations) != 4 {
		t.Fatalf("expected 4 healthy installations (codex, gemini, opencode, cursor), got %d", len(stEnv.Installations))
	}

	// 5. Remove Cursor -> personal.mdc preserved, 3 other runtimes stay healthy!
	depsRm, _ := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"remove", "engineer", "--project", "--runtime", "cursor", "--yes"}, depsRm); err != nil {
		t.Fatalf("remove cursor failed: %v", err)
	}
	if gotRule, err := os.ReadFile(userRulePath); err != nil || string(gotRule) != userRule {
		t.Fatalf("personal.mdc altered on cursor removal: %q err=%v", string(gotRule), err)
	}
	if _, err := os.Stat(filepath.Join(proj, ".cursor", "rules", "dtkit-engineer.mdc")); !os.IsNotExist(err) {
		t.Fatalf("expected dtkit-engineer.mdc removed")
	}

	depsAfter, outAfter := makeDeps(true)
	if err := RunWithDeps(context.Background(), []string{"status", "--project", "--json"}, depsAfter); err != nil {
		t.Fatalf("status after cursor removal failed: %v\n%s", err, outAfter.String())
	}
	var afterEnv Envelope
	if err := json.Unmarshal(outAfter.Bytes(), &afterEnv); err != nil {
		t.Fatal(err)
	}
	if len(afterEnv.Installations) != 3 {
		t.Fatalf("expected 3 remaining installations (codex+gemini+opencode), got %d", len(afterEnv.Installations))
	}
}
