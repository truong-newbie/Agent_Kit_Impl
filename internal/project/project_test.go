package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestParseAndValidateSecurityAndMalformedCases(t *testing.T) {
	validJSON := `{
  "schemaVersion": 1,
  "name": "sample",
  "roots": [{"path": ".", "stack": "go"}],
  "checks": [
    {"id": "go-test", "kind": "test", "executable": "go", "argv": ["test", "./..."], "cwd": ".", "timeoutSec": 120}
  ]
}`
	p, err := ParseAndValidate([]byte(validJSON))
	if err != nil {
		t.Fatalf("expected valid profile to parse, got %v", err)
	}
	dig1 := CommandsTrustDigest(p)

	// CommandsTrustDigest must change when argv changes (trust invalidation).
	p2 := p
	p2.Checks = []CheckSpec{{ID: "go-test", Kind: "test", Executable: "go", Argv: []string{"test", "-run", "Foo", "./..."}, Cwd: ".", TimeoutSec: 120}}
	if dig2 := CommandsTrustDigest(p2); dig2 == dig1 {
		t.Fatal("expected CommandsTrustDigest to invalidate when check argv changes")
	}

	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"malformed", "{"},
		{"trailing_json", validJSON + "\n{}"},
		{"unknown_field", `{"schemaVersion":1,"extra":true,"roots":[],"checks":[]}`},
		{"bad_schema_ver", `{"schemaVersion":2,"roots":[],"checks":[]}`},
		{"duplicate_check_id", `{"schemaVersion":1,"roots":[],"checks":[{"id":"a","kind":"test","executable":"go","argv":[],"cwd":".","timeoutSec":10},{"id":"a","kind":"test","executable":"go","argv":[],"cwd":".","timeoutSec":10}]}`},
		{"shell_wrapper_sh", `{"schemaVersion":1,"roots":[],"checks":[{"id":"sh","kind":"test","executable":"sh","argv":["-c","echo pwned"],"cwd":".","timeoutSec":10}]}`},
		{"shell_wrapper_cmd", `{"schemaVersion":1,"roots":[],"checks":[{"id":"cmd","kind":"test","executable":"cmd.exe","argv":["/c","dir"],"cwd":".","timeoutSec":10}]}`},
		{"shell_wrapper_pwsh", `{"schemaVersion":1,"roots":[],"checks":[{"id":"ps","kind":"test","executable":"powershell","argv":["-Command","ls"],"cwd":".","timeoutSec":10}]}`},
		{"abs_exe_path", `{"schemaVersion":1,"roots":[],"checks":[{"id":"abs","kind":"test","executable":"/usr/bin/go","argv":["test"],"cwd":".","timeoutSec":10}]}`},
		{"win_abs_exe_path", `{"schemaVersion":1,"roots":[],"checks":[{"id":"winabs","kind":"test","executable":"C:\\Go\\bin\\go.exe","argv":["test"],"cwd":".","timeoutSec":10}]}`},
		{"cwd_traversal", `{"schemaVersion":1,"roots":[],"checks":[{"id":"trav","kind":"test","executable":"go","argv":["test"],"cwd":"../outside","timeoutSec":10}]}`},
		{"windows_drive_cwd", `{"schemaVersion":1,"roots":[],"checks":[{"id":"drv","kind":"test","executable":"go","argv":["test"],"cwd":"C:\\repo","timeoutSec":10}]}`},
		{"secret_allow_env", `{"schemaVersion":1,"roots":[],"checks":[{"id":"sec","kind":"test","executable":"go","argv":["test"],"cwd":".","timeoutSec":10,"allowEnv":["ANTHROPIC_API_KEY"]}]}`},
		{"ld_preload_env", `{"schemaVersion":1,"roots":[],"checks":[{"id":"ld","kind":"test","executable":"go","argv":["test"],"cwd":".","timeoutSec":10,"allowEnv":["LD_PRELOAD"]}]}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseAndValidate([]byte(tc.raw)); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			} else if apperr.ExitCodeOf(err) != apperr.ExitUsage {
				t.Fatalf("expected ExitUsage for %s, got %v", tc.name, err)
			}
		})
	}
}

func TestInspectMonorepoReadOnlyAndInitSafety(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(base, "monorepo")
	home := filepath.Join(base, "home")
	for _, d := range []string{
		filepath.Join(repo, "services", "api"),
		filepath.Join(repo, "apps", "web"),
		filepath.Join(repo, "tools", "py", "tests"),
		filepath.Join(repo, ".github", "workflows"),
		home,
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Root go.work + service go.mod + pnpm monorepo workspace + Python tool + Makefile + CI workflow.
	files := map[string]string{
		"go.work":                  "go 1.27.4\nuse ./services/api\n",
		"services/api/go.mod":      "module example.com/api\ngo 1.27.4\n",
		"pnpm-lock.yaml":           "lockfileVersion: '9.0'\n",
		"package.json":             `{"name":"mono","workspaces":["apps/*"],"scripts":{"lint":"echo COULD_EXECUTE_IF_UNSAFE"}}`,
		"apps/web/package.json":    `{"name":"web","scripts":{"test":"vitest run","typecheck":"tsc --noEmit"}}`,
		"apps/web/tsconfig.json":   `{}`,
		"tools/py/pyproject.toml":  "[project]\nname='pytool'\n[tool.pytest.ini_options]\n[tool.ruff]\n[tool.mypy]\n",
		"Makefile":                 "test:\n\ttouch MUST_NOT_RUN\nlint:\n\techo lint\n",
		".github/workflows/ci.yml": "name: CI\n",
	}
	for rel, content := range files {
		p := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.WriteFile(p, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	env := paths.Environment{
		Getwd:       func() (string, error) { return repo, nil },
		UserHomeDir: func() (string, error) { return home, nil },
		Getenv:      func(string) string { return "" },
	}

	insp, err := Inspect(context.Background(), env, repo)
	if err != nil {
		t.Fatalf("Inspect failed: %v", err)
	}
	if insp.ProfileExists {
		t.Fatal("expected ProfileExists=false initially")
	}
	if _, err := os.Stat(filepath.Join(repo, "MUST_NOT_RUN")); err == nil {
		t.Fatal("Inspect executed Makefile test target unexpectedly")
	}
	if _, err := os.Stat(filepath.Join(repo, ".dtkit")); !os.IsNotExist(err) {
		t.Fatal("Inspect created .dtkit directory unexpectedly")
	}

	rawProf, _ := json.Marshal(insp.Profile)
	text := string(rawProf)
	for _, want := range []string{`"stack":"go"`, `"stack":"ts"`, `"stack":"python"`, `"services/api"`, `"apps/web"`, `"tools/py"`, `"pnpm"`, `"pytest"`, `"ruff"`, `"mypy"`, `"services-api-go-test"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected inspected profile to contain %s, got %s", want, text)
		}
	}

	// Dry-run Init writes zero files.
	dryRes, err := Init(context.Background(), env, InitOptions{Path: repo, DryRun: true})
	if err != nil || dryRes.Created || dryRes.PreviewJSON == "" {
		t.Fatalf("dry-run init failed: %+v err=%v", dryRes, err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".dtkit")); !os.IsNotExist(err) {
		t.Fatal("dry-run init created .dtkit on disk")
	}

	// Real Init writes .dtkit/project.json cleanly.
	applyRes, err := Init(context.Background(), env, InitOptions{Path: repo, DryRun: false})
	if err != nil || !applyRes.Created {
		t.Fatalf("apply init failed: %+v err=%v", applyRes, err)
	}
	loaded, _, exists, err := LoadProfile(repo)
	if err != nil || !exists || CommandsTrustDigest(loaded) != applyRes.TrustDigest {
		t.Fatalf("reloaded profile mismatch: exists=%v dig=%s err=%v", exists, CommandsTrustDigest(loaded), err)
	}

	// User edits profile -> second Init refuses overwrite cleanly.
	userEdit := strings.Replace(applyRes.PreviewJSON, `"timeoutSec": 120`, `"timeoutSec": 300`, 1)
	if err := os.WriteFile(applyRes.ConfigPath, []byte(userEdit), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(context.Background(), env, InitOptions{Path: repo}); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected Conflict refusing overwrite, got %v", err)
	}
	afterRaw, _ := os.ReadFile(applyRes.ConfigPath)
	if string(afterRaw) != userEdit {
		t.Fatal("user-edited .dtkit/project.json was overwritten")
	}

	// Symlink check on Unix/capable systems.
	if runtime.GOOS != "windows" {
		symRepo := filepath.Join(base, "symrepo")
		if err := os.MkdirAll(filepath.Join(symRepo, ".dtkit"), 0755); err != nil {
			t.Fatal(err)
		}
		outside := filepath.Join(base, "outside.json")
		_ = os.WriteFile(outside, []byte(applyRes.PreviewJSON), 0644)
		if err := os.Symlink(outside, filepath.Join(symRepo, ".dtkit", "project.json")); err == nil {
			symEnv := paths.Environment{Getwd: func() (string, error) { return symRepo, nil }, UserHomeDir: func() (string, error) { return home, nil }}
			if _, err := Inspect(context.Background(), symEnv, symRepo); err == nil {
				t.Fatal("expected symlinked .dtkit/project.json to be rejected")
			}
		}
	}
}
