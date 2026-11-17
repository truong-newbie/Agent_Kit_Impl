package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestCLIProjectInspectAndInitWorkflow(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(base, "repo")
	home := filepath.Join(base, "home")
	for _, d := range []string{repo, home} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/demo\ngo 1.27.4\n"), 0644); err != nil {
		t.Fatal(err)
	}

	makeDeps := func() (Deps, *bytes.Buffer, *bytes.Buffer) {
		var out, errBuf bytes.Buffer
		return Deps{
			Stdin:  &bytes.Buffer{},
			Stdout: &out,
			Stderr: &errBuf,
			Env: paths.Environment{
				Getwd:       func() (string, error) { return repo, nil },
				UserHomeDir: func() (string, error) { return home, nil },
				Getenv:      func(string) string { return "" },
			},
			IsInteractive: func() bool { return false },
		}, &out, &errBuf
	}

	// 1. project inspect --json
	deps, out, _ := makeDeps()
	if err := RunWithDeps(context.Background(), []string{"project", "inspect", "--path", repo, "--json"}, deps); err != nil {
		t.Fatalf("project inspect failed: %v\n%s", err, out.String())
	}
	var inspEnv Envelope
	if err := json.Unmarshal(out.Bytes(), &inspEnv); err != nil || !inspEnv.Ok || inspEnv.ProjectInspect == nil {
		t.Fatalf("unexpected inspect envelope: %v (%s)", err, out.String())
	}
	if len(inspEnv.ProjectInspect.Profile.Checks) < 2 {
		t.Fatalf("expected detected go checks, got %+v", inspEnv.ProjectInspect.Profile.Checks)
	}

	// 2. project init --dry-run --json leaves zero disk files
	deps, out, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"project", "init", "--path", repo, "--dry-run", "--json"}, deps); err != nil {
		t.Fatalf("project init --dry-run failed: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".dtkit")); !os.IsNotExist(err) {
		t.Fatal("project init --dry-run must not create .dtkit")
	}

	// 3. project init --json without --yes requires confirmation
	deps, out, _ = makeDeps()
	err = RunWithDeps(context.Background(), []string{"project", "init", "--path", repo, "--json"}, deps)
	if apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected ExitUsage when --yes omitted in --json mode, got %v", err)
	}

	// 4. project init --yes --json writes .dtkit/project.json
	deps, out, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"project", "init", "--path", repo, "--yes", "--json"}, deps); err != nil {
		t.Fatalf("project init --yes failed: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(repo, ".dtkit", "project.json")); err != nil {
		t.Fatalf("expected .dtkit/project.json to exist: %v", err)
	}

	// 5. Re-running project init conflicts instead of clobbering existing profile
	deps, out, _ = makeDeps()
	err = RunWithDeps(context.Background(), []string{"project", "init", "--path", repo, "--yes", "--json"}, deps)
	if apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict on duplicate project init, got %v (%s)", err, out.String())
	}
}
