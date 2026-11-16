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

func TestOpenCodeLifecycleSharedAgentsMDAndThreeWayCoexistence(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(base, "codex-home")
	geminiBase := filepath.Join(base, "gemini-cli-home")
	ocDir := filepath.Join(base, "opencode-cfg")
	proj := filepath.Join(base, "repo")
	for _, d := range []string{home, codexHome, geminiBase, ocDir, proj} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	userAgents := []byte("\xef\xbb\xbf# Shared Team Rules\r\nKeep commits small.\r\n")
	if err := os.WriteFile(filepath.Join(proj, "AGENTS.md"), userAgents, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "opencode.json"), []byte(`{invalid-json`), 0644); err != nil {
		t.Fatal(err)
	}

	makeDeps := func(hasOpenCode bool, ocOverride string) (Deps, *bytes.Buffer) {
		deps, _, _ := testDeps("", false, home, codexHome, proj, true)
		var outBuf bytes.Buffer
		deps.Stdout = &outBuf
		prevGetenv := deps.Env.Getenv
		deps.Env.Getenv = func(k string) string {
			switch k {
			case "GEMINI_CLI_HOME":
				return geminiBase
			case "OPENCODE_CONFIG_DIR":
				return ocOverride
			default:
				return prevGetenv(k)
			}
		}
		deps.LookPath = func(file string) (string, error) {
			if file == "codex" || file == "gemini" {
				return "/usr/local/bin/" + file, nil
			}
			if file == "opencode" && hasOpenCode {
				return "/usr/local/bin/opencode", nil
			}
			return "", os.ErrNotExist
		}
		return deps, &outBuf
	}

	// 1. doctor --runtime opencode warns on invalid opencode.json and returns exit 4 without binary.
	depsMissing, outMissing := makeDeps(false, ocDir)
	err := RunWithDeps(context.Background(), []string{"doctor", "--runtime", "opencode", "--project", "--json"}, depsMissing)
	if apperr.ExitCodeOf(err) != apperr.ExitRuntime {
		t.Fatalf("expected ExitRuntime when opencode missing, got %d (%v)", apperr.ExitCodeOf(err), err)
	}
	var docEnv Envelope
	if err := json.Unmarshal(outMissing.Bytes(), &docEnv); err != nil {
		t.Fatal(err)
	}
	sawBadConfig := false
	for _, c := range docEnv.Checks {
		if c.Name == "opencode_config" && c.Status == "warn" {
			sawBadConfig = true
		}
	}
	if !sawBadConfig {
		t.Fatalf("expected invalid opencode.json warning, got %+v", docEnv.Checks)
	}

	// Fix opencode.json cleanly before install/status checks.
	if err := os.WriteFile(filepath.Join(proj, "opencode.json"), []byte(`{"instructions":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Install Codex, Gemini, AND OpenCode into the SAME project!
	for _, rt := range []string{"codex", "gemini", "opencode"} {
		d, _ := makeDeps(true, ocDir)
		if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--runtime", rt, "--yes"}, d); err != nil {
			t.Fatalf("install %s project failed: %v", rt, err)
		}
	}
	// Verify shared AGENTS.md has both Codex & OpenCode blocks and kept UTF-8 BOM + CRLF.
	agentsBytes, err := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(agentsBytes, userAgents) || !bytes.Contains(agentsBytes, []byte("<!-- dtkit:begin engineer project -->")) || !bytes.Contains(agentsBytes, []byte("<!-- dtkit:begin engineer opencode project -->")) {
		t.Fatalf("shared AGENTS.md missing user header or both runtime blocks:\n%q", string(agentsBytes))
	}

	// 3. Install OpenCode globally into OPENCODE_CONFIG_DIR and verify status across all 3 runtimes.
	depsGlob, _ := makeDeps(true, ocDir)
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--global", "--runtime", "opencode", "--yes"}, depsGlob); err != nil {
		t.Fatalf("global opencode install failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ocDir, "skills", "dtkit-opencode-global-plan", "SKILL.md")); err != nil {
		t.Fatalf("missing global opencode skill: %v", err)
	}

	depsSt, outSt := makeDeps(true, ocDir)
	if err := RunWithDeps(context.Background(), []string{"status", "--project", "--json"}, depsSt); err != nil {
		t.Fatalf("status --project failed: %v\n%s", err, outSt.String())
	}
	var stEnv Envelope
	if err := json.Unmarshal(outSt.Bytes(), &stEnv); err != nil {
		t.Fatal(err)
	}
	if len(stEnv.Installations) != 3 {
		t.Fatalf("expected 3 healthy installations (codex, gemini, opencode), got %d: %+v", len(stEnv.Installations), stEnv.Installations)
	}

	// 4. OPENCODE_CONFIG_DIR relocation triggers profile_mismatch.
	depsMis, _ := makeDeps(true, filepath.Join(base, "other-oc"))
	err = RunWithDeps(context.Background(), []string{"status", "--global", "--runtime", "opencode", "--json"}, depsMis)
	if apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected profile_mismatch conflict on changed OPENCODE_CONFIG_DIR, got %v", err)
	}

	// 5. Remove OpenCode -> Codex block in AGENTS.md AND Gemini files remain healthy and intact!
	depsRmOC, _ := makeDeps(true, ocDir)
	if err := RunWithDeps(context.Background(), []string{"remove", "engineer", "--project", "--runtime", "opencode", "--yes"}, depsRmOC); err != nil {
		t.Fatalf("remove opencode failed: %v", err)
	}
	afterAgents, err := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(afterAgents, userAgents) || !bytes.Contains(afterAgents, []byte("<!-- dtkit:begin engineer project -->")) || strings.Contains(string(afterAgents), "dtkit:begin engineer opencode") {
		t.Fatalf("after opencode removal, AGENTS.md corrupted:\n%q", string(afterAgents))
	}

	// Codex + Gemini still healthy after removing OpenCode.
	depsAfter, outAfter := makeDeps(true, ocDir)
	if err := RunWithDeps(context.Background(), []string{"status", "--project", "--json"}, depsAfter); err != nil {
		t.Fatalf("status after opencode removal failed: %v\n%s", err, outAfter.String())
	}
	var afterEnv Envelope
	if err := json.Unmarshal(outAfter.Bytes(), &afterEnv); err != nil {
		t.Fatal(err)
	}
	if len(afterEnv.Installations) != 2 {
		t.Fatalf("expected 2 remaining installations (codex+gemini), got %d", len(afterEnv.Installations))
	}
}
