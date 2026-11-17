package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/check"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestCLICheckDryRunTrustAndExecution(t *testing.T) {
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
	_ = os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/chkcli\ngo 1.27.4\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "main_test.go"), []byte("package chkcli\nimport \"testing\"\nfunc TestOK(t *testing.T){}\n"), 0644)

	makeDeps := func() (Deps, *bytes.Buffer, *bytes.Buffer) {
		var out, errBuf bytes.Buffer
		return Deps{
			Stdin:         &bytes.Buffer{},
			Stdout:        &out,
			Stderr:        &errBuf,
			IsInteractive: func() bool { return false },
			Env: paths.Environment{
				Getwd:       func() (string, error) { return repo, nil },
				UserHomeDir: func() (string, error) { return home, nil },
			},
			LookPath: func(file string) (string, error) {
				if file == "go" {
					return exec.LookPath("go")
				}
				return "", errors.New("not found")
			},
		}, &out, &errBuf
	}

	// Initialize project profile first.
	deps, _, _ := makeDeps()
	if err := RunWithDeps(context.Background(), []string{"project", "init", "--path", repo, "--yes", "--json"}, deps); err != nil {
		t.Fatalf("project init failed: %v", err)
	}

	// 1. dtkit check --dry-run --json previews checks without trust confirmation or lock mutation.
	deps, out, _ := makeDeps()
	if err := RunWithDeps(context.Background(), []string{"check", "--path", repo, "--all", "--dry-run", "--json"}, deps); err != nil {
		t.Fatalf("check --dry-run failed: %v\n%s", err, out.String())
	}
	var dryEnv Envelope
	if err := json.Unmarshal(out.Bytes(), &dryEnv); err != nil || !dryEnv.Ok || dryEnv.CheckReport == nil || dryEnv.CheckReport.Summary.Skipped == 0 {
		t.Fatalf("unexpected dry-run envelope: %v (%s)", err, out.String())
	}

	// 2. Real check without --yes rejects untrusted checks.
	deps, out, _ = makeDeps()
	err = RunWithDeps(context.Background(), []string{"check", "--path", repo, "--all", "--json"}, deps)
	if apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected Conflict on untrusted check execution, got %v (%s)", err, out.String())
	}

	// 3. Real check with --yes executes go vet + go test cleanly.
	deps, out, _ = makeDeps()
	if err := RunWithDeps(context.Background(), []string{"check", "--path", repo, "--all", "--yes", "--json"}, deps); err != nil {
		t.Fatalf("check --all --yes failed: %v\n%s", err, out.String())
	}
	var runEnv Envelope
	if err := json.Unmarshal(out.Bytes(), &runEnv); err != nil || !runEnv.Ok || runEnv.CheckReport == nil {
		t.Fatalf("unexpected check run envelope: %v (%s)", err, out.String())
	}
	if runEnv.CheckReport.Summary.Pass < 2 || runEnv.CheckReport.Results[0].Status != check.StatusPass {
		t.Fatalf("expected go checks passing, got %+v", runEnv.CheckReport)
	}
}
