package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestCLIScoutGuardHookAndMCPWorkflow(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	repo := filepath.Join(base, "polyrepo")

	files := map[string]string{
		"frontend/package.json": `{"name":"web","scripts":{"test":"echo ok"}}`,
		"frontend/src/app.ts":   "export const ok = true;\n",
		"backend/pom.xml":       "<project></project>\n",
		"backend/Auth.java":     "public class Auth {}\n",
		"ai/pyproject.toml":     "[project]\nname = \"ai\"\n",
		"ai/worker.py":          "def run():\n    return 1\n",
	}
	for rel, body := range files {
		full := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}

	env := paths.Environment{
		Getwd:       func() (string, error) { return repo, nil },
		UserHomeDir: func() (string, error) { return home, nil },
	}
	run := func(stdin string, args ...string) (Envelope, string, error) {
		var out, errBuf bytes.Buffer
		deps := DefaultDeps(strings.NewReader(stdin), &out, &errBuf)
		deps.Env = env
		err := RunWithDeps(context.Background(), args, deps)
		var parsed Envelope
		_ = json.Unmarshal(out.Bytes(), &parsed)
		return parsed, out.String() + errBuf.String(), err
	}

	// 1. dtkit scout --write-summary --json
	scoutEnv, scoutRaw, err := run("", "scout", "--path", repo, "--write-summary", "--json")
	if err != nil {
		t.Fatalf("dtkit scout failed: %v (%s)", err, scoutRaw)
	}
	if scoutEnv.Scout == nil || scoutEnv.Scout.LanguageStats["java"] != 1 || scoutEnv.Scout.LanguageStats["python"] != 1 {
		t.Fatalf("unexpected scout result: %+v", scoutEnv.Scout)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "codebase-summary.md")); err != nil {
		t.Fatalf("expected docs/codebase-summary.md written: %v", err)
	}

	// 2. dtkit guard lock --mode read-only blocks guard-edit hook
	lockEnv, lockRaw, err := run("", "guard", "lock", "--path", repo, "--mode", "read-only", "--json")
	if err != nil || lockEnv.GuardStatus == nil || lockEnv.GuardStatus.Policy.Mode != "read-only" {
		t.Fatalf("guard lock read-only failed: %v (%s)", err, lockRaw)
	}

	_, hookDenyRaw, err := run(`{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":"backend/Auth.java"}}`,
		"hook", "run", "guard-edit", "--scope", "project", "--root", repo, "--runtime", "claude")
	if err != nil || !strings.Contains(hookDenyRaw, `"permissionDecision":"deny"`) {
		t.Fatalf("expected Claude guard-edit deny in read-only mode, got err=%v out=%s", err, hookDenyRaw)
	}

	// Gemini BeforeTool format deny too
	_, geminiDenyRaw, err := run(`{"hook_event_name":"BeforeTool","tool_name":"WriteFile","tool_input":{"file_path":"ai/worker.py"}}`,
		"hook", "run", "guard-edit", "--scope", "project", "--root", repo, "--runtime", "gemini")
	if err != nil || !strings.Contains(geminiDenyRaw, `"decision":"deny"`) {
		t.Fatalf("expected Gemini guard-edit deny in read-only mode, got err=%v out=%s", err, geminiDenyRaw)
	}

	// 3. tdd-red permits test files only, tdd-green protects test files
	if _, raw, err := run("", "guard", "lock", "--path", repo, "--mode", "tdd-red", "--json"); err != nil {
		t.Fatalf("guard lock tdd-red failed: %v (%s)", err, raw)
	}
	_, testAllowRaw, err := run(`{"hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"ai/test_worker.py"}}`,
		"hook", "run", "guard-edit", "--scope", "project", "--root", repo, "--runtime", "claude")
	if err != nil || strings.Contains(testAllowRaw, `"deny"`) {
		t.Fatalf("expected tdd-red to allow ai/test_worker.py, got err=%v out=%s", err, testAllowRaw)
	}
	_, srcDenyRaw, err := run(`{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":"ai/worker.py"}}`,
		"hook", "run", "guard-edit", "--scope", "project", "--root", repo, "--runtime", "claude")
	if err != nil || !strings.Contains(srcDenyRaw, `"deny"`) {
		t.Fatalf("expected tdd-red to deny ai/worker.py, got err=%v out=%s", err, srcDenyRaw)
	}

	if _, raw, err := run("", "guard", "unlock", "--path", repo, "--json"); err != nil {
		t.Fatalf("guard unlock failed: %v (%s)", err, raw)
	}

	// 4. guard verify fails closed before passing check
	_, _, verifyErr := run("", "guard", "verify", "--path", repo, "--json")
	if apperr.ExitCodeOf(verifyErr) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict on unverified guard verify, got %v", verifyErr)
	}

	// Create a passing trusted .dtkit/project.json check (`go version` is always available in Docker / CI)
	goBin, lpErr := exec.LookPath("go")
	_ = goBin
	if lpErr == nil {
		profileJSON := `{"schemaVersion":1,"name":"polyrepo","roots":[{"path":".","stack":"generic"}],"checks":[{"id":"go-ver","kind":"test","executable":"go","argv":["version"],"cwd":".","timeoutSec":30}]}`
		if err := os.MkdirAll(filepath.Join(repo, ".dtkit"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, ".dtkit", "project.json"), []byte(profileJSON), 0644); err != nil {
			t.Fatal(err)
		}
		if _, chkRaw, err := run("", "check", "--path", repo, "--all", "--yes", "--json"); err != nil {
			t.Fatalf("dtkit check --all --yes failed: %v (%s)", err, chkRaw)
		}
		vEnv, vRaw, err := run("", "guard", "verify", "--path", repo, "--json")
		if err != nil || vEnv.GuardStatus == nil || !vEnv.GuardStatus.Verification.Verified {
			t.Fatalf("expected guard verify pass after passing check, got err=%v out=%s", err, vRaw)
		}

		// Edit code -> receipt invalidated
		if err := os.WriteFile(filepath.Join(repo, "ai", "worker.py"), []byte("def run():\n    return 2\n"), 0644); err != nil {
			t.Fatal(err)
		}
		_, _, vAfterErr := run("", "guard", "verify", "--path", repo, "--json")
		if apperr.ExitCodeOf(vAfterErr) != apperr.ExitConflict {
			t.Fatalf("expected ExitConflict after edit invalidated receipt, got %v", vAfterErr)
		}
	}

	// 5. dtkit mcp serve via RunWithDeps
	mcpIn := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"dtkit_scout","arguments":{"query":"worker"}}}` + "\n"
	_, mcpRaw, err := run(mcpIn, "mcp", "serve", "--path", repo)
	if err != nil {
		t.Fatalf("dtkit mcp serve failed: %v (%s)", err, mcpRaw)
	}
	if !strings.Contains(mcpRaw, "dtkit_guard_verify") || !strings.Contains(mcpRaw, "ai/worker.py") {
		t.Fatalf("unexpected mcp serve output:\n%s", mcpRaw)
	}
}
