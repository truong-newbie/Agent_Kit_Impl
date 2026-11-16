package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func testDeps(stdin string, interactive bool, home string, codexHome string, cwd string, codexExists bool) (Deps, *bytes.Buffer, *bytes.Buffer) {
	var out, errBuf bytes.Buffer
	deps := Deps{
		Stdin:         strings.NewReader(stdin),
		Stdout:        &out,
		Stderr:        &errBuf,
		IsInteractive: func() bool { return interactive },
		Env: paths.Environment{
			Getwd:       func() (string, error) { return cwd, nil },
			UserHomeDir: func() (string, error) { return home, nil },
			Getenv: func(k string) string {
				if k == "CODEX_HOME" {
					return codexHome
				}
				return ""
			},
			GitTopLevel: func(context.Context, string) (string, error) { return "", errors.New("not git") },
		},
		Now: func() time.Time { return time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC) },
		LookPath: func(name string) (string, error) {
			if codexExists && name == "codex" {
				return "/usr/local/bin/codex", nil
			}
			return "", errors.New("not found")
		},
	}
	return deps, &out, &errBuf
}

func TestVersionAndHelpAndUnimplemented(t *testing.T) {
	var out, errBuf bytes.Buffer
	if err := Run([]string{"version", "--json"}, &out, &errBuf); err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if !env.Ok || env.BuildInfo == nil || env.BuildInfo.Version == "" {
		t.Fatalf("unexpected version envelope: %+v", env)
	}

	out.Reset()
	err := Run([]string{"unknown-cmd", "--json"}, &out, &errBuf)
	if apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected usage exit code for unknown command, got %d (%v)", apperr.ExitCodeOf(err), err)
	}
	var errEnv Envelope
	if jsonErr := json.Unmarshal(out.Bytes(), &errEnv); jsonErr != nil {
		t.Fatalf("JSON error output invalid: %v (%s)", jsonErr, out.String())
	}
	if errEnv.Ok || len(errEnv.Errors) != 1 {
		t.Fatalf("unexpected error envelope: %+v", errEnv)
	}
}

func TestListAndInfo(t *testing.T) {
	for _, args := range [][]string{{"list"}, {"info", "engineer"}} {
		var out, stderr bytes.Buffer
		if err := Run(args, &out, &stderr); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(out.String(), "engineer") {
			t.Fatalf("missing engineer: %s", out.String())
		}
	}
}

func TestInstallDryRunIsReadOnly(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := Run([]string{"install", "engineer", "--project", "--path", root, "--dry-run", "--json"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.DryRun || !env.Ok || env.Summary == nil || env.Summary.Create != 15 {
		t.Fatalf("unexpected preview envelope: %+v", env)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("preview wrote files: %v %v", entries, err)
	}
}

func TestInstallConfirmationAndCancellation(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	_ = os.MkdirAll(home, 0755)

	// Non-interactive without --yes returns ExitUsage (2).
	deps, _, _ := testDeps("", false, home, "", root, true)
	err = RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--path", root}, deps)
	if apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected ExitUsage, got %d (%v)", apperr.ExitCodeOf(err), err)
	}

	// Interactive decline returns ExitCancelled (130) and writes nothing.
	deps, _, _ = testDeps("n\n", true, home, "", root, true)
	err = RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--path", root}, deps)
	if apperr.ExitCodeOf(err) != apperr.ExitCancelled {
		t.Fatalf("expected ExitCancelled, got %d (%v)", apperr.ExitCodeOf(err), err)
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Fatalf("declined install wrote files: %v", entries)
	}

	// Cancelled context returns ExitCancelled (130).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = RunWithDeps(ctx, []string{"install", "engineer", "--project", "--path", root, "--yes"}, deps)
	if apperr.ExitCodeOf(err) != apperr.ExitCancelled {
		t.Fatalf("expected ExitCancelled for cancelled ctx, got %d (%v)", apperr.ExitCodeOf(err), err)
	}
}

func TestFreshProjectInstallAndUnicodePath(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "dự án test")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("personal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := Run([]string{"install", "engineer", "--project", "--path", root, "--yes"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Installed engineer") {
		t.Fatalf("unexpected output: %s", out.String())
	}
	content, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), "personal\n") || !strings.Contains(string(content), "dtkit:begin") {
		t.Fatalf("personal instructions were not preserved: %s", content)
	}
	if _, err := os.Stat(filepath.Join(root, ".dtkit", "state.json")); err != nil {
		t.Fatal(err)
	}
}

func TestStatusReportsBothScopesAndExitCode3OnDrift(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(base, "codex-home")
	proj := filepath.Join(base, "project")
	for _, d := range []string{home, codexHome, proj} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	deps, out, _ := testDeps("", false, home, codexHome, proj, false)
	if err := RunWithDeps(context.Background(), []string{"status", "--json"}, deps); err != nil {
		t.Fatal(err)
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Installations) != 2 || env.Installations[0].Installed || env.Installations[1].Installed {
		t.Fatalf("expected 2 uninstalled reports, got %+v", env.Installations)
	}

	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--yes"}, deps); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"status", "--project"}, deps); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "healthy") {
		t.Fatalf("expected healthy status: %s", out.String())
	}

	if err := os.WriteFile(filepath.Join(proj, ".agents", "skills", "dtkit-project-plan", "SKILL.md"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	err = RunWithDeps(context.Background(), []string{"status", "--project"}, deps)
	if apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict (3) on drift, got %d (%v)", apperr.ExitCodeOf(err), err)
	}
	if !strings.Contains(out.String(), "drift") || !strings.Contains(out.String(), "modified") {
		t.Fatalf("expected drift status: %s", out.String())
	}
}

func TestDoctorExitCodes(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	proj := filepath.Join(base, "proj")
	for _, d := range []string{home, proj} {
		_ = os.MkdirAll(d, 0755)
	}

	depsMissing, out, _ := testDeps("", false, home, "", proj, false)
	err = RunWithDeps(context.Background(), []string{"doctor", "--json"}, depsMissing)
	if apperr.ExitCodeOf(err) != apperr.ExitRuntime {
		t.Fatalf("expected ExitRuntime (4) when codex missing, got %d (%v)", apperr.ExitCodeOf(err), err)
	}
	var env Envelope
	if jsonErr := json.Unmarshal(out.Bytes(), &env); jsonErr != nil {
		t.Fatalf("invalid doctor json: %v\n%s", jsonErr, out.String())
	}
	if env.Ok || len(env.Checks) == 0 {
		t.Fatalf("unexpected doctor envelope: %+v", env)
	}

	depsReady, out2, _ := testDeps("", false, home, "", proj, true)
	if err := RunWithDeps(context.Background(), []string{"doctor"}, depsReady); err != nil {
		t.Fatalf("expected ready doctor, got %v (%s)", err, out2.String())
	}
}

func TestValidateLocalPackageDirectory(t *testing.T) {
	pkgDir := filepath.Join("..", "catalog", "engineer")
	var out, errBuf bytes.Buffer
	if err := Run([]string{"validate", pkgDir, "--json"}, &out, &errBuf); err != nil {
		t.Fatalf("validate failed: %v\n%s", err, out.String())
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Ok || env.Kit == nil || env.Kit.ID != "engineer" {
		t.Fatalf("unexpected validate envelope: %+v", env)
	}
}

func TestRecoverCommandNoopAndDryRun(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	if err := Run([]string{"recover", "--project", "--path", root, "--dry-run", "--json"}, &out, &errBuf); err != nil {
		t.Fatalf("recover dry-run failed: %v\n%s", err, out.String())
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Ok || env.Recovery == nil || env.Recovery.Outcome != "noop" {
		t.Fatalf("unexpected recover envelope: %+v", env)
	}
}

func TestInstallIdempotentNoopAndBothScopesCoexistence(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(base, "codex")
	proj := filepath.Join(base, "proj")
	for _, d := range []string{home, codexHome, proj} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(proj, "AGENTS.md"), []byte("\xef\xbb\xbf# Project\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	deps, out, _ := testDeps("", false, home, codexHome, proj, true)
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--yes"}, deps); err != nil {
		t.Fatalf("project install failed: %v", err)
	}
	statePath := filepath.Join(proj, ".dtkit", "state.json")
	stateBefore, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	txBefore, _ := os.ReadDir(filepath.Join(proj, ".dtkit", "local", "transactions"))

	// Second install without --yes is an absolute noop (no state rewrite, no new tx, exits 0).
	deps.Now = func() time.Time { return time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC) }
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--json"}, deps); err != nil {
		t.Fatalf("repeat install should be noop: %v (%s)", err, out.String())
	}
	stateAfter, _ := os.ReadFile(statePath)
	if !bytes.Equal(stateBefore, stateAfter) {
		t.Fatal("idempotent install modified state.json")
	}
	txAfter, _ := os.ReadDir(filepath.Join(proj, ".dtkit", "local", "transactions"))
	if len(txBefore) != len(txAfter) {
		t.Fatal("idempotent install created new transaction")
	}

	// Global install coexists independently.
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--global", "--yes"}, deps); err != nil {
		t.Fatalf("global install failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(codexHome, "AGENTS.md")); err != nil {
		t.Fatalf("missing global AGENTS.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dtkit-global-plan", "SKILL.md")); err != nil {
		t.Fatalf("missing global skill: %v", err)
	}

	// Status without scope reports both installations healthy.
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"status", "--json"}, deps); err != nil {
		t.Fatalf("dual status failed: %v", err)
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Installations) != 2 || !env.Installations[0].Healthy || !env.Installations[1].Healthy {
		t.Fatalf("unexpected dual status: %+v", env.Installations)
	}

	// Remove project without affecting global.
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"remove", "engineer", "--project", "--yes", "--json"}, deps); err != nil {
		t.Fatalf("project remove failed: %v (%s)", err, out.String())
	}
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"status", "--json"}, deps); err != nil {
		t.Fatalf("status after remove failed: %v", err)
	}
	var envAfter Envelope
	if err := json.Unmarshal(out.Bytes(), &envAfter); err != nil {
		t.Fatal(err)
	}
	if envAfter.Installations[0].Installed || !envAfter.Installations[1].Installed || !envAfter.Installations[1].Healthy {
		t.Fatalf("expected project removed and global untouched: %+v", envAfter.Installations)
	}
}

func TestUpdateCLIFromLocalFixture(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj")
	home := filepath.Join(base, "home")
	pkgV2Dir := filepath.Join(base, "kit-v2")
	for _, d := range []string{proj, home, filepath.Join(pkgV2Dir, "rules"), filepath.Join(pkgV2Dir, "skills", "plan")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	manifest := `{
  "schemaVersion": 1,
  "id": "engineer",
  "name": "DTKit Engineer Kit",
  "version": "0.3.0",
  "description": "Updated kit",
  "runtimeTargets": ["codex"],
  "scopes": ["project", "global"],
  "requiresCapabilities": ["instructions", "skills"],
  "packageDependencies": [],
  "components": [
    {"id":"rule/engineering","kind":"rule","source":"rules/engineering.md","requires":[]},
    {"id":"skill/plan","kind":"skill","source":"skills/plan","requires":["rule/engineering"]}
  ]
}`
	_ = os.WriteFile(filepath.Join(pkgV2Dir, "package.json"), []byte(manifest), 0644)
	_ = os.WriteFile(filepath.Join(pkgV2Dir, "rules", "engineering.md"), []byte("# Updated Rules\n"), 0644)
	_ = os.WriteFile(filepath.Join(pkgV2Dir, "skills", "plan", "SKILL.md.tmpl"), []byte("---\nname: {{.SkillName}}\ndescription: Updated plan\n---\n# Plan v2\n"), 0644)

	deps, out, _ := testDeps("", false, home, "", proj, true)
	if err := RunWithDeps(context.Background(), []string{"install", "engineer", "--project", "--yes"}, deps); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := RunWithDeps(context.Background(), []string{"update", "engineer", "--project", "--from", pkgV2Dir, "--yes", "--json"}, deps); err != nil {
		t.Fatalf("cli update failed: %v\n%s", err, out.String())
	}
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Ok || env.Generation != 2 || env.Kit == nil || env.Kit.Version != "0.3.0" {
		t.Fatalf("unexpected update envelope: %+v", env)
	}
}
