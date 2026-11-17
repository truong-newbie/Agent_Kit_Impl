package check

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
)

func TestCheckTrustInvalidationMissingToolFailOutputCapAndLock(t *testing.T) {
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

	env := paths.Environment{
		Getwd:       func() (string, error) { return repo, nil },
		UserHomeDir: func() (string, error) { return home, nil },
	}

	// 1. Missing .dtkit/project.json returns Conflict.
	if _, err := Run(context.Background(), env, Options{Path: repo}); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected missing_project_profile conflict, got %v", err)
	}

	// Write a valid Go fixture + multi-check profile.
	_ = os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/chk\ngo 1.27.4\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "ok_test.go"), []byte("package chk\nimport \"testing\"\nfunc TestPass(t *testing.T){}\n"), 0644)

	pyExe := "python3"
	if _, lpErr := exec.LookPath("python3"); lpErr != nil {
		if _, lp2 := exec.LookPath("python"); lp2 == nil {
			pyExe = "python"
		} else {
			pyExe = ""
		}
	}

	checks := []project.CheckSpec{
		{ID: "go-test", Kind: "test", Executable: "go", Argv: []string{"test", "./..."}, Cwd: ".", TimeoutSec: 60},
		{ID: "missing-lint", Kind: "lint", Executable: "dtkit-missing-linter-xyz", Argv: []string{"--check"}, Cwd: ".", TimeoutSec: 30},
		{ID: "autofix-fmt", Kind: "format", Executable: "go", Argv: []string{"fmt", "./..."}, Cwd: ".", TimeoutSec: 30, MutatesFiles: true},
	}
	if pyExe != "" {
		checks = append(checks,
			project.CheckSpec{ID: "big-output", Kind: "custom", Executable: pyExe, Argv: []string{"-c", "print('A' * 100000)"}, Cwd: ".", TimeoutSec: 30},
			project.CheckSpec{ID: "py-fail", Kind: "custom", Executable: pyExe, Argv: []string{"-c", "import sys; print('bad'); sys.exit(2)"}, Cwd: ".", TimeoutSec: 30},
		)
	}

	prof := project.Profile{
		SchemaVersion: 1,
		Name:          "chk",
		Roots:         []project.WorkspaceRoot{{Path: ".", Stack: "go"}},
		Checks:        checks,
	}
	writeProfile := func(p project.Profile) {
		raw, _ := json.MarshalIndent(p, "", "  ")
		_ = os.MkdirAll(filepath.Join(repo, ".dtkit"), 0755)
		if err := os.WriteFile(filepath.Join(repo, ".dtkit", "project.json"), append(raw, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
	writeProfile(prof)

	// 2. Dry-run works without prior trust and never executes or writes trust file.
	dryRep, err := Run(context.Background(), env, Options{Path: repo, DryRun: true, All: true})
	if err != nil || dryRep.Trusted || dryRep.Summary.Skipped != len(checks) {
		t.Fatalf("unexpected dry-run report: %+v err=%v", dryRep, err)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".dtkit", "local", "check-trust.json")); !os.IsNotExist(statErr) {
		t.Fatal("dry-run check created check-trust.json")
	}

	// 3. Real run without --yes refuses untrusted checks.
	if _, err := Run(context.Background(), env, Options{Path: repo, All: true}); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected untrusted_project_checks conflict, got %v", err)
	}

	// 4. Real run with --yes records trust and separates pass, missing_tool, fail, truncated, and autofix skip.
	rep, err := Run(context.Background(), env, Options{Path: repo, All: true, Yes: true})
	if err != nil {
		t.Fatalf("check run failed: %v", err)
	}
	if rep.Ok || !rep.Trusted {
		t.Fatalf("expected Ok=false (due to missing_tool/fail) and Trusted=true, got %+v", rep)
	}
	wantPass := 1
	if pyExe != "" {
		wantPass = 2
	}
	if rep.Summary.Pass != wantPass || rep.Summary.MissingTool != 1 || rep.Summary.Skipped != 1 {
		t.Fatalf("unexpected summary counts (wantPass=%d): %+v results=%+v", wantPass, rep.Summary, rep.Results)
	}
	for _, r := range rep.Results {
		if r.ID == "big-output" && (!r.Truncated || !strings.HasSuffix(r.Output, "... [truncated]") || len(r.Output) > MaxOutputBytes+64) {
			t.Fatalf("expected capped truncated output for big-output, got len=%d truncated=%v", len(r.Output), r.Truncated)
		}
		if r.ID == "py-fail" && (r.Status != StatusFail || r.ExitCode != 2) {
			t.Fatalf("expected py-fail exit 2 fail, got %+v", r)
		}
	}

	// 5. Subsequent run without --yes succeeds while trust digest matches; mutating check invalidates trust!
	prof.Checks[0].Argv = []string{"test", "-v", "./..."}
	writeProfile(prof)
	if _, err := Run(context.Background(), env, Options{Path: repo, All: true}); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected trust invalidation after check edit, got %v", err)
	}

	// 6. Active lock rejects overlapping check execution.
	_ = os.MkdirAll(filepath.Join(repo, ".dtkit", "local"), 0700)
	_ = os.WriteFile(filepath.Join(repo, ".dtkit", "local", "check.lock"), []byte(`{"pid":1}`), 0600)
	if _, err := Run(context.Background(), env, Options{Path: repo, All: true, Yes: true}); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected check_lock_held conflict, got %v", err)
	}
	_ = os.Remove(filepath.Join(repo, ".dtkit", "local", "check.lock"))
}

func TestCheckTimeoutKillsProcessTreeAndChangedMonorepoSelection(t *testing.T) {
	pyExe := "python3"
	if _, lpErr := exec.LookPath("python3"); lpErr != nil {
		if _, lp2 := exec.LookPath("python"); lp2 == nil {
			pyExe = "python"
		} else {
			pyExe = ""
		}
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(base, "mono")
	home := filepath.Join(base, "home")
	for _, d := range []string{
		filepath.Join(repo, "services", "api"),
		filepath.Join(repo, "apps", "web"),
		home,
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(repo, "services", "api", "go.mod"), []byte("module example.com/api\ngo 1.27.4\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "services", "api", "api_test.go"), []byte("package api\nimport \"testing\"\nfunc TestOK(t *testing.T){}\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "apps", "web", "app.txt"), []byte("v1\n"), 0644)

	checks := []project.CheckSpec{
		{ID: "api-test", Kind: "test", Executable: "go", Argv: []string{"test", "./..."}, Cwd: "services/api", TimeoutSec: 60},
		{ID: "web-lint", Kind: "lint", Executable: "dtkit-web-lint-missing", Argv: []string{}, Cwd: "apps/web", TimeoutSec: 60},
	}
	if pyExe != "" {
		checks = append(checks, project.CheckSpec{
			ID: "slow-tree", Kind: "custom", Executable: pyExe,
			Argv: []string{"-c", "import subprocess,sys,time; subprocess.Popen([sys.executable,'-c','import time; time.sleep(30)']); time.sleep(30)"},
			Cwd:  "services/api", TimeoutSec: 1,
		})
	}
	prof := project.Profile{
		SchemaVersion: 1,
		Name:          "mono",
		Roots: []project.WorkspaceRoot{
			{Path: "services/api", Stack: "go"},
			{Path: "apps/web", Stack: "node"},
		},
		Checks: checks,
	}
	raw, _ := json.MarshalIndent(prof, "", "  ")
	_ = os.MkdirAll(filepath.Join(repo, ".dtkit"), 0755)
	_ = os.WriteFile(filepath.Join(repo, ".dtkit", "project.json"), append(raw, '\n'), 0644)

	env := paths.Environment{Getwd: func() (string, error) { return repo, nil }, UserHomeDir: func() (string, error) { return home, nil }}

	// Non-git fallback runs all checks and enforces 1s timeout + child tree kill promptly.
	start := time.Now()
	nonGitRep, err := Run(context.Background(), env, Options{Path: repo, ChangedOnly: true, Yes: true})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if nonGitRep.Changed == nil || !nonGitRep.Changed.FallbackAll {
		t.Fatalf("expected non-git changed fallbackAll=true, got %+v", nonGitRep.Changed)
	}
	if pyExe != "" {
		if elapsed > 8*time.Second {
			t.Fatalf("timeout took too long (%s); child process tree may not have been killed", elapsed)
		}
		foundTO := false
		for _, r := range nonGitRep.Results {
			if r.ID == "slow-tree" && r.Status == StatusTimeout {
				foundTO = true
			}
		}
		if !foundTO {
			t.Fatalf("expected slow-tree status=timeout, got %+v", nonGitRep.Results)
		}
	}

	// Git repo: only modify apps/web/app.txt -> api-test skipped, web-lint selected!
	if _, gitErr := exec.LookPath("git"); gitErr == nil {
		gitRun := func(args ...string) {
			c := exec.Command("git", append([]string{"-C", repo}, args...)...)
			if out, e := c.CombinedOutput(); e != nil {
				t.Fatalf("git %v: %v\n%s", args, e, out)
			}
		}
		gitRun("init", "-q")
		gitRun("config", "user.name", "T")
		gitRun("config", "user.email", "t@example.com")
		gitRun("add", ".")
		gitRun("commit", "-q", "-m", "init")
		_ = os.WriteFile(filepath.Join(repo, "apps", "web", "app.txt"), []byte("v2\n"), 0644)

		chRep, err := Run(context.Background(), env, Options{Path: repo, ChangedOnly: true, DryRun: true})
		if err != nil {
			t.Fatal(err)
		}
		statusByID := map[string]CheckResult{}
		for _, r := range chRep.Results {
			statusByID[r.ID] = r
		}
		if statusByID["api-test"].Reason != "workspace untouched in changed files" || statusByID["web-lint"].Reason != "dry_run" {
			t.Fatalf("unexpected changed monorepo selection: %+v", statusByID)
		}
	}
}
