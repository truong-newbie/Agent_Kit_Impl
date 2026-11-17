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
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func TestCLITeamExportImportTemplateAndUnifiedAllRuntimesStatus(t *testing.T) {
	// --- Workspace A (Alice): setup Go project profile, engineer@minimal, hooks, lockfile, trust ---
	homeA := filepath.Join(t.TempDir(), "alice-home")
	projA := filepath.Join(homeA, "src", "service-a")
	for _, d := range []string{homeA, projA} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(projA, "go.mod"), []byte("module example.com/svc\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Canary script in Makefile MUST NOT run during export/import/template!
	canary := filepath.Join(projA, "CANARY_RAN.txt")
	if err := os.WriteFile(filepath.Join(projA, "Makefile"), []byte("test:\n\t@touch CANARY_RAN.txt\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runIn := func(home, proj string, args ...string) (Envelope, string, error) {
		var out, errBuf bytes.Buffer
		deps := DefaultDeps(nil, &out, &errBuf)
		deps.Env = paths.Environment{
			Getwd:       func() (string, error) { return proj, nil },
			UserHomeDir: func() (string, error) { return home, nil },
			Getenv:      func(k string) string { return "" },
		}
		deps.LookPath = func(file string) (string, error) { return "/usr/bin/" + file, nil }
		err := RunWithDeps(context.Background(), args, deps)
		var resp Envelope
		if bytes.Contains(out.Bytes(), []byte(`"schemaVersion"`)) {
			_ = json.Unmarshal(out.Bytes(), &resp)
		}
		return resp, out.String(), err
	}

	// 1. Unified cross-runtime inspect report lists all 5 runtimes cleanly.
	allSt, allRaw, err := runIn(homeA, projA, "status", "--all-runtimes", "--project", "--redact", "--json")
	if err != nil || len(allSt.Installations) != 5 {
		t.Fatalf("status --all-runtimes expected 5 runtime entries, got len=%d err=%v (%s)", len(allSt.Installations), err, allRaw)
	}
	if strings.Contains(allRaw, "alice-home") {
		t.Fatalf("status --redact leaked home path:\n%s", allRaw)
	}

	// 2. Scaffold repo onboarding template cleanly (dry-run first, then apply) without running Makefile.
	if _, raw, err := runIn(homeA, projA, "team", "template", "--preset", "minimal", "--dry-run", "--json"); err != nil {
		t.Fatalf("team template dry-run failed: %v (%s)", err, raw)
	}
	if _, err := os.Stat(filepath.Join(projA, "dtkit.lock")); !os.IsNotExist(err) {
		t.Fatalf("team template --dry-run must not write dtkit.lock")
	}

	tplEnv, tplRaw, err := runIn(homeA, projA, "team", "template", "--preset", "minimal", "--yes", "--json")
	if err != nil || tplEnv.TeamTemplate == nil || len(tplEnv.TeamTemplate.WrittenFiles) != 3 {
		t.Fatalf("team template failed: err=%v (%s)", err, tplRaw)
	}
	// Running template a second time skips existing files safely.
	tpl2Env, _, err := runIn(homeA, projA, "team", "template", "--preset", "minimal", "--yes", "--json")
	if err != nil || len(tpl2Env.TeamTemplate.SkippedFiles) != 3 {
		t.Fatalf("expected 3 skipped existing files on 2nd team template: %+v", tpl2Env.TeamTemplate)
	}

	// Seed machine-local .dtkit/local/ state & check-trust on Machine A to confirm export never leaks them.
	if err := os.MkdirAll(filepath.Join(projA, ".dtkit", "local"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projA, ".dtkit", "local", "check-trust.json"), []byte(`{"secretLocalTrust":"machine-a-only"}`), 0644); err != nil {
		t.Fatal(err)
	}

	bundleOut := filepath.Join(t.TempDir(), "portable-team.json")
	expEnv, expRaw, err := runIn(homeA, projA, "team", "export", "--name", "service-team", "--runtimes", "codex,claude", "--hooks", "check-changed,guard-edit", "--out", bundleOut, "--yes", "--json")
	if err != nil || expEnv.TeamBundle == nil {
		t.Fatalf("team export failed: err=%v (%s)", err, expRaw)
	}
	exportedBytes, err := os.ReadFile(bundleOut)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"alice-home", homeA, projA, "machine-a-only", "state.json"} {
		if strings.Contains(string(exportedBytes), leak) {
			t.Fatalf("team export leaked non-portable token %q:\n%s", leak, string(exportedBytes))
		}
	}

	// --- Workspace B (Bob, CI clean checkout): different home/workspace imports bundle ---
	homeB := filepath.Join(t.TempDir(), "bob-home")
	projB := filepath.Join(homeB, "ci", "clean-checkout")
	for _, d := range []string{homeB, projB} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Incompatible runtime requested fails closed BEFORE writing anything.
	_, _, badRTErr := runIn(homeB, projB, "team", "import", bundleOut, "--runtime", "gemini", "--yes")
	if apperr.ExitCodeOf(badRTErr) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict for runtime not in team bundle, got %v", badRTErr)
	}
	if _, err := os.Stat(filepath.Join(projB, "dtkit.lock")); !os.IsNotExist(err) {
		t.Fatalf("failed runtime compatibility must not write dtkit.lock")
	}

	// Dry-run import writes nothing and checks project trust boundary.
	prevImp, prevRaw, err := runIn(homeB, projB, "team", "import", bundleOut, "--runtime", "claude", "--dry-run", "--diff", "--json")
	if err != nil || prevImp.TeamImport == nil || len(prevImp.SyncResults) != 1 {
		t.Fatalf("team import dry-run failed: err=%v (%s)", err, prevRaw)
	}
	if _, err := os.Stat(filepath.Join(projB, ".dtkit", "project.json")); !os.IsNotExist(err) {
		t.Fatalf("team import --dry-run must not write .dtkit/project.json")
	}

	// Real import writes dtkit.lock + .dtkit/project.json, installs claude kit, does NOT write check-trust.json, and does NOT run commands.
	impEnv, impRaw, err := runIn(homeB, projB, "team", "import", bundleOut, "--runtime", "claude", "--yes", "--json")
	if err != nil || impEnv.TeamImport == nil {
		t.Fatalf("team import apply failed: err=%v (%s)", err, impRaw)
	}
	if _, err := os.Stat(filepath.Join(projB, ".claude", "skills", "dtkit-claude-project-ask", "SKILL.md")); err != nil {
		t.Fatalf("expected claude minimal skill installed by team import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projB, ".dtkit", "local", "check-trust.json")); !os.IsNotExist(err) {
		t.Fatalf("team import MUST NOT auto-trust .dtkit/project.json checks")
	}
	if _, err := os.Stat(canary); !os.IsNotExist(err) {
		t.Fatalf("canary Makefile target should never have executed")
	}

	// Verify unified status across all runtimes shows claude installed while others remain uninstalled.
	bobStatus, bobRaw, err := runIn(homeB, projB, "status", "--all-runtimes", "--project", "--json")
	if err != nil || len(bobStatus.Installations) != 5 {
		t.Fatalf("bob status --all-runtimes failed: err=%v (%s)", err, bobRaw)
	}
	installedRuntimes := 0
	for _, r := range bobStatus.Installations {
		if r.Installed {
			installedRuntimes++
		}
	}
	if installedRuntimes != 1 {
		t.Fatalf("expected exactly 1 installed runtime after claude import, got %d", installedRuntimes)
	}
}
