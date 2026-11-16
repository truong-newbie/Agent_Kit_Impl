package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHooksCLICycleAndExecution(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(base, "codex")
	proj := filepath.Join(base, "proj dir")
	stableExe := filepath.Join(base, "bin", "dtkit")
	for _, d := range []string{home, codexHome, proj, filepath.Dir(stableExe)} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(stableExe, []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}

	deps, out, _ := testDeps("", false, home, codexHome, proj, true)
	deps.Executable = func() (string, error) { return stableExe, nil }

	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--yes"}, deps); err != nil {
		t.Fatal(err)
	}

	// Enable dry-run does not create hooks.json.
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"hooks", "enable", "session-context", "--project", "--dry-run", "--json"}, deps); err != nil {
		t.Fatalf("hooks enable dry-run failed: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(proj, ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatal("dry-run wrote hooks.json")
	}

	// Real enable.
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"hooks", "enable", "session-context", "--project", "--yes", "--json"}, deps); err != nil {
		t.Fatalf("hooks enable failed: %v\n%s", err, out.String())
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Ok || len(env.Hooks) != 1 || env.Hooks[0].Status != "configured_pending_trust" {
		t.Fatalf("unexpected enable envelope: %+v", env)
	}

	// Execute `dtkit hook run session-context` directly via CLI.
	out.Reset()
	deps.Stdin = strings.NewReader(`{"hook_event_name":"SessionStart","source":"startup"}`)
	if err := RunWithDeps(context.Background(), []string{"hook", "run", "session-context", "--scope", "project", "--root", proj}, deps); err != nil {
		t.Fatalf("hook run failed: %v", err)
	}
	if !strings.Contains(out.String(), `"hookEventName":"SessionStart"`) || !strings.Contains(out.String(), "dtkit-project-plan") {
		t.Fatalf("unexpected hook run stdout: %s", out.String())
	}

	// Disable removes DTKit-created hooks.json.
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"hooks", "disable", "session-context", "--project", "--yes"}, deps); err != nil {
		t.Fatalf("hooks disable failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, ".codex", "hooks.json")); !os.IsNotExist(err) {
		t.Fatalf("DTKit-created hooks.json survived disable: %v", err)
	}
	_ = bytes.Compare
}
