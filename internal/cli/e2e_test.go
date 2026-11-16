package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuiltBinaryEndToEndLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping built-binary E2E test in short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("go toolchain not on PATH: %v", err)
	}
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exeName := "dtkit"
	if runtime.GOOS == "windows" {
		exeName = "dtkit.exe"
	}
	binPath := filepath.Join(binDir, exeName)
	buildCmd := exec.Command(goBin, "build", "-o", binPath, "./cmd/dtkit")
	buildCmd.Dir = repoRoot
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build dtkit binary: %v\n%s", err, string(out))
	}

	sandbox, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(sandbox, "home")
	codexHome := filepath.Join(sandbox, "codex-home")
	proj := filepath.Join(sandbox, "my project")
	emptyPathDir := filepath.Join(sandbox, "empty-bin")
	v2Dir := filepath.Join(sandbox, "kit-v2")
	for _, d := range []string{home, codexHome, proj, emptyPathDir, filepath.Join(v2Dir, "rules"), filepath.Join(v2Dir, "skills", "plan")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	env := append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"CODEX_HOME="+codexHome,
		"PATH="+emptyPathDir,
	)

	runBin := func(cwd string, args ...string) (int, string, string) {
		cmd := exec.Command(binPath, args...)
		cmd.Dir = cwd
		cmd.Env = env
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err == nil {
			return 0, stdout.String(), stderr.String()
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), stdout.String(), stderr.String()
		}
		t.Fatalf("exec %v failed unexpectedly: %v", args, err)
		return -1, "", ""
	}

	// 1. version --json & list --json & validate
	code, out, _ := runBin(proj, "version", "--json")
	if code != 0 || !strings.Contains(out, `"ok":true`) {
		t.Fatalf("version failed: code=%d out=%s", code, out)
	}
	code, out, _ = runBin(proj, "validate", filepath.Join(repoRoot, "internal", "catalog", "engineer"), "--json")
	if code != 0 || !strings.Contains(out, `"engineer"`) {
		t.Fatalf("validate failed: code=%d out=%s", code, out)
	}

	// 2. doctor fails with exit 4 when codex is missing from PATH.
	code, out, _ = runBin(proj, "doctor", "--json")
	if code != 4 {
		t.Fatalf("expected doctor exit code 4 without codex, got %d (%s)", code, out)
	}

	// 3. non-interactive install without --yes returns exit 2.
	code, out, _ = runBin(proj, "install", "engineer", "--project", "--json")
	if code != 2 {
		t.Fatalf("expected exit 2 without --yes, got %d (%s)", code, out)
	}
	var errEnv Envelope
	if err := json.Unmarshal([]byte(out), &errEnv); err != nil || errEnv.Ok {
		t.Fatalf("invalid JSON error output: %v (%s)", err, out)
	}

	// 4. dry-run writes zero files.
	code, _, _ = runBin(proj, "install", "engineer", "--project", "--dry-run")
	if code != 0 {
		t.Fatalf("dry-run failed: code=%d", code)
	}
	if entries, _ := os.ReadDir(proj); len(entries) != 0 {
		t.Fatalf("dry-run wrote files: %v", entries)
	}

	// 5. install project + global, repeat install noop without --yes.
	code, out, _ = runBin(proj, "install", "engineer", "--project", "--yes")
	if code != 0 || !strings.Contains(out, "Installed engineer") {
		t.Fatalf("project install failed: code=%d out=%s", code, out)
	}
	code, out, _ = runBin(proj, "install", "engineer", "--project")
	if code != 0 || !strings.Contains(out, "up to date") {
		t.Fatalf("idempotent repeat install failed: code=%d out=%s", code, out)
	}
	code, _, _ = runBin(proj, "install", "engineer", "--global", "--yes")
	if code != 0 {
		t.Fatalf("global install failed: code=%d", code)
	}

	// 6. status healthy -> drift returns exit 3.
	code, _, _ = runBin(proj, "status")
	if code != 0 {
		t.Fatalf("status healthy failed: code=%d", code)
	}
	skillFile := filepath.Join(proj, ".agents", "skills", "dtkit-project-plan", "SKILL.md")
	origSkill, _ := os.ReadFile(skillFile)
	_ = os.WriteFile(skillFile, []byte("drifted"), 0644)
	code, _, _ = runBin(proj, "status", "--project")
	if code != 3 {
		t.Fatalf("expected exit 3 on drift, got %d", code)
	}
	_ = os.WriteFile(skillFile, origSkill, 0644)

	// 7. update from local v0.3.0 kit.
	manifest := `{"schemaVersion":1,"id":"engineer","name":"DTKit Engineer Kit","version":"0.3.0","description":"v3","runtimeTargets":["codex"],"scopes":["project","global"],"requiresCapabilities":["instructions","skills"],"packageDependencies":[],"components":[{"id":"rule/engineering","kind":"rule","source":"rules/engineering.md","requires":[]},{"id":"skill/plan","kind":"skill","source":"skills/plan","requires":["rule/engineering"]}]}`
	_ = os.WriteFile(filepath.Join(v2Dir, "package.json"), []byte(manifest), 0644)
	_ = os.WriteFile(filepath.Join(v2Dir, "rules", "engineering.md"), []byte("# Rules v2\n"), 0644)
	_ = os.WriteFile(filepath.Join(v2Dir, "skills", "plan", "SKILL.md.tmpl"), []byte("---\nname: {{.SkillName}}\ndescription: v2 plan\n---\n# Plan v2\n"), 0644)
	code, out, _ = runBin(proj, "update", "engineer", "--project", "--from", v2Dir, "--yes")
	if code != 0 || !strings.Contains(out, "0.2.0 -> 0.3.0") {
		t.Fatalf("update failed: code=%d out=%s", code, out)
	}

	// 7b. Enable opt-in SessionStart hook, execute the generated native shell command against the built binary, then verify remove cleans it up.
	code, out, _ = runBin(proj, "hooks", "enable", "session-context", "--project", "--executable", binPath, "--yes", "--json")
	if code != 0 || !strings.Contains(out, `"configured_pending_trust"`) {
		t.Fatalf("hooks enable failed: code=%d out=%s", code, out)
	}
	var hookEnv Envelope
	if err := json.Unmarshal([]byte(out), &hookEnv); err != nil || len(hookEnv.Hooks) != 1 {
		t.Fatalf("unexpected hooks enable json: %v (%s)", err, out)
	}
	shellCmd := hookEnv.Hooks[0].Command
	var execHook *exec.Cmd
	if runtime.GOOS == "windows" {
		shellCmd = hookEnv.Hooks[0].CommandWindows
		execHook = exec.Command("powershell", "-NoProfile", "-Command", shellCmd)
	} else {
		execHook = exec.Command("sh", "-c", shellCmd)
	}
	execHook.Env = env
	execHook.Stdin = strings.NewReader(`{"hook_event_name":"SessionStart","source":"startup","cwd":"/ignored"}`)
	hookStdout, hookErr := execHook.CombinedOutput()
	if hookErr != nil || !strings.Contains(string(hookStdout), `"SessionStart"`) || !strings.Contains(string(hookStdout), "dtkit-project-plan") {
		t.Fatalf("native hook execution failed: err=%v cmd=%s out=%s", hookErr, shellCmd, string(hookStdout))
	}

	// 8. remove project cleans up kit and hook entry, leaving global healthy.
	code, _, _ = runBin(proj, "remove", "engineer", "--project", "--yes")
	if code != 0 {
		t.Fatalf("remove failed: code=%d", code)
	}
	code, out, _ = runBin(proj, "status", "--global", "--json")
	if code != 0 || !strings.Contains(out, `"healthy":true`) {
		t.Fatalf("global status after project remove failed: code=%d out=%s", code, out)
	}
	code, out, _ = runBin(proj, "recover", "--project", "--dry-run", "--json")
	if code != 0 || !strings.Contains(out, `"outcome":"noop"`) {
		t.Fatalf("recover noop failed: code=%d out=%s", code, out)
	}
}
