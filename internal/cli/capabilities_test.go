package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

func TestCapabilityContractAndConsistentWarningsAcrossCommands(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj")
	home := filepath.Join(base, "home")
	strictKitDir := filepath.Join(base, "strict-kit")
	hooksKitDir := filepath.Join(base, "hooks-kit")
	v3KitDir := filepath.Join(base, "eng-v3")
	for _, d := range []string{
		proj,
		home,
		filepath.Join(strictKitDir, "rules"),
		filepath.Join(strictKitDir, "skills", "plan"),
		filepath.Join(hooksKitDir, "rules"),
		filepath.Join(hooksKitDir, "skills", "plan"),
		filepath.Join(v3KitDir, "rules"),
		filepath.Join(v3KitDir, "skills", "plan"),
	} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Strict kit requiring "instructions" MUST fail pre-write on cursor global (--dry-run, --yes, and validate).
	strictManifest := `{
  "schemaVersion": 1,
  "id": "strict-kit",
  "name": "Strict Kit",
  "version": "0.1.0",
  "description": "Requires instructions everywhere",
  "runtimeTargets": ["codex", "cursor"],
  "scopes": ["project", "global"],
  "requiresCapabilities": ["instructions", "skills"],
  "packageDependencies": [],
  "components": [
    {"id":"rule/engineering","kind":"rule","source":"rules/engineering.md","requires":[]},
    {"id":"skill/plan","kind":"skill","source":"skills/plan","requires":["rule/engineering"]}
  ]
}`
	_ = os.WriteFile(filepath.Join(strictKitDir, "package.json"), []byte(strictManifest), 0644)
	_ = os.WriteFile(filepath.Join(strictKitDir, "rules", "engineering.md"), []byte("# Rule\n"), 0644)
	_ = os.WriteFile(filepath.Join(strictKitDir, "skills", "plan", "SKILL.md.tmpl"), []byte("---\nname: {{.SkillName}}\ndescription: Strict\n---\n# Plan\n"), 0644)

	run := func(args ...string) (Envelope, string, int, error) {
		deps, out, _ := testDeps("", false, home, "", proj, false) // codex/cursor binary absent on PATH
		err := RunWithDeps(context.Background(), args, deps)
		var env Envelope
		_ = json.Unmarshal(out.Bytes(), &env)
		return env, out.String(), apperr.ExitCodeOf(err), err
	}

	beforeHome := snapshotRoot(t, home)
	envStrictDry, rawStrictDry, code, _ := run("install", "strict-kit", "--from", strictKitDir, "--runtime", "cursor", "--global", "--dry-run", "--json")
	if code != apperr.ExitUsage || envStrictDry.Ok || len(envStrictDry.Errors) == 0 || envStrictDry.Errors[0].Code != "unsupported_capability" {
		t.Fatalf("expected unsupported_capability on strict cursor/global dry-run, got code=%d raw=%s", code, rawStrictDry)
	}
	envStrictYes, rawStrictYes, code, _ := run("install", "strict-kit", "--from", strictKitDir, "--runtime", "cursor", "--global", "--yes", "--json")
	if code != apperr.ExitUsage || envStrictYes.Ok || !reflect.DeepEqual(beforeHome, snapshotRoot(t, home)) {
		t.Fatalf("strict cursor/global install failed or wrote files unexpectedly: code=%d raw=%s", code, rawStrictYes)
	}
	if _, rawVal, valCode, _ := run("validate", strictKitDir, "--json"); valCode != apperr.ExitUsage || !strings.Contains(rawVal, "unsupported_capability") {
		t.Fatalf("expected validate to catch unsupported cursor/global instructions requirement, got code=%d raw=%s", valCode, rawVal)
	}

	// 2. Kit requiring "hooks" fails pre-write across runtimes (e.g. opencode/codex).
	hooksManifest := strings.ReplaceAll(strictManifest, `"id": "strict-kit"`, `"id": "hooks-kit"`)
	hooksManifest = strings.ReplaceAll(hooksManifest, `"runtimeTargets": ["codex", "cursor"]`, `"runtimeTargets": ["codex", "opencode"]`)
	hooksManifest = strings.ReplaceAll(hooksManifest, `"requiresCapabilities": ["instructions", "skills"]`, `"requiresCapabilities": ["skills", "hooks"]`)
	_ = os.WriteFile(filepath.Join(hooksKitDir, "package.json"), []byte(hooksManifest), 0644)
	_ = os.WriteFile(filepath.Join(hooksKitDir, "rules", "engineering.md"), []byte("# Rule\n"), 0644)
	_ = os.WriteFile(filepath.Join(hooksKitDir, "skills", "plan", "SKILL.md.tmpl"), []byte("---\nname: {{.SkillName}}\ndescription: Hook kit\n---\n# Plan\n"), 0644)

	for _, rt := range []string{"codex", "opencode"} {
		_, rawH, hCode, _ := run("install", "hooks-kit", "--from", hooksKitDir, "--runtime", rt, "--project", "--yes", "--json")
		if hCode != apperr.ExitUsage || !strings.Contains(rawH, "unsupported_capability") {
			t.Fatalf("expected hooks unsupported_capability on %s, got code=%d raw=%s", rt, hCode, rawH)
		}
	}

	// 3. Consistent warnings, capabilities, and integration status across info, dry-run, apply, noop, update, status, and doctor for Cursor global + absent binary.
	wantBinaryWarn := "cursor executable not found on PATH; files will still be installed"
	wantFallbackWarn := "Cursor global User Rules are configured in Cursor Settings UI; DTKit installs global skills (~/.cursor/skills/) and resources only"
	assertConsistentWarningsAndIntegration := func(label string, env Envelope) {
		t.Helper()
		var hasBin, hasFallback bool
		for _, w := range env.Warnings {
			if w == wantBinaryWarn {
				hasBin = true
			}
			if w == wantFallbackWarn {
				hasFallback = true
			}
		}
		if !hasBin || !hasFallback {
			t.Fatalf("%s: expected both binary and global fallback warnings, got %+v", label, env.Warnings)
		}
		if env.Capabilities == nil || env.Capabilities.Instructions.DTKitSupported || !env.Capabilities.Skills.DTKitSupported {
			t.Fatalf("%s: unexpected capabilities: %+v", label, env.Capabilities)
		}
		if env.Integration == nil || env.Integration.Status != adapter.IntegrationUnverified {
			t.Fatalf("%s: expected unverified integration metadata, got %+v", label, env.Integration)
		}
	}

	envInfo, _, code, _ := run("info", "engineer", "--runtime", "cursor", "--global", "--json")
	if code != 0 {
		t.Fatalf("info cursor/global failed: code=%d", code)
	}
	assertConsistentWarningsAndIntegration("info", envInfo)

	envDry, _, code, _ := run("install", "engineer", "--runtime", "cursor", "--global", "--dry-run", "--json")
	if code != 0 || !reflect.DeepEqual(beforeHome, snapshotRoot(t, home)) {
		t.Fatalf("dry-run cursor/global failed or mutated home: code=%d", code)
	}
	assertConsistentWarningsAndIntegration("install_dry_run", envDry)

	envApply, _, code, _ := run("install", "engineer", "--runtime", "cursor", "--global", "--yes", "--json")
	if code != 0 {
		t.Fatalf("apply cursor/global failed: code=%d", code)
	}
	assertConsistentWarningsAndIntegration("install_apply", envApply)

	envNoop, _, code, _ := run("install", "engineer", "--runtime", "cursor", "--global", "--yes", "--json")
	if code != 0 {
		t.Fatalf("noop install cursor/global failed: code=%d", code)
	}
	assertConsistentWarningsAndIntegration("install_noop", envNoop)

	envUpNoop, _, code, _ := run("update", "engineer", "--runtime", "cursor", "--global", "--yes", "--json")
	if code != 0 {
		t.Fatalf("noop update cursor/global failed: code=%d", code)
	}
	assertConsistentWarningsAndIntegration("update_noop", envUpNoop)

	// Upgrade v0.3.0 with optionalCapabilities also shows consistent warnings in dry-run and apply.
	v3Manifest := `{
  "schemaVersion": 1,
  "id": "engineer",
  "name": "DTKit Engineer Kit",
  "version": "0.3.0",
  "description": "v3",
  "runtimeTargets": ["cursor"],
  "scopes": ["global"],
  "requiresCapabilities": ["skills"],
  "optionalCapabilities": ["instructions"],
  "packageDependencies": [],
  "components": [
    {"id":"rule/engineering","kind":"rule","source":"rules/engineering.md","requires":[]},
    {"id":"skill/plan","kind":"skill","source":"skills/plan","requires":["rule/engineering"]}
  ]
}`
	_ = os.WriteFile(filepath.Join(v3KitDir, "package.json"), []byte(v3Manifest), 0644)
	_ = os.WriteFile(filepath.Join(v3KitDir, "rules", "engineering.md"), []byte("# Rule v3\n"), 0644)
	_ = os.WriteFile(filepath.Join(v3KitDir, "skills", "plan", "SKILL.md.tmpl"), []byte("---\nname: {{.SkillName}}\ndescription: Plan v3\n---\n# Plan v3\n"), 0644)

	envUpDry, _, code, _ := run("update", "engineer", "--from", v3KitDir, "--runtime", "cursor", "--global", "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("update dry-run cursor/global failed: code=%d", code)
	}
	assertConsistentWarningsAndIntegration("update_dry_run", envUpDry)

	envUpApply, _, code, _ := run("update", "engineer", "--from", v3KitDir, "--runtime", "cursor", "--global", "--yes", "--json")
	if code != 0 {
		t.Fatalf("update apply cursor/global failed: code=%d", code)
	}
	assertConsistentWarningsAndIntegration("update_apply", envUpApply)

	// 4. Status and doctor cleanly separate file health vs integration status without network requests.
	envSt, stTextErr := func() (Envelope, string) {
		e, _, stCode, _ := run("status", "--runtime", "cursor", "--global", "--json")
		if stCode != 0 {
			t.Fatalf("status cursor/global json failed: code=%d", stCode)
		}
		depsText, outText, _ := testDeps("", false, home, "", proj, false)
		if err := RunWithDeps(context.Background(), []string{"status", "--runtime", "cursor", "--global"}, depsText); err != nil {
			t.Fatalf("status cursor/global text failed: %v", err)
		}
		return e, outText.String()
	}()
	assertConsistentWarningsAndIntegration("status", envSt)
	if len(envSt.Installations) != 1 || !envSt.Installations[0].Healthy || envSt.Installations[0].IntegrationStatus != adapter.IntegrationUnverified {
		t.Fatalf("unexpected status report separation of healthy vs integrationStatus: %+v", envSt.Installations)
	}
	if !strings.Contains(stTextErr, "artifact files are healthy") || !strings.Contains(stTextErr, "integration: unverified") {
		t.Fatalf("expected status text to distinguish file health from integration status, got %q", stTextErr)
	}

	envDoc, _, docCode, _ := run("doctor", "--runtime", "cursor", "--global", "--json")
	if docCode != apperr.ExitRuntime {
		t.Fatalf("expected ExitRuntime=4 when cursor binary missing, got %d", docCode)
	}
	var sawIntegWarn, sawInstrWarn bool
	for _, c := range envDoc.Checks {
		if c.Name == "runtime_integration" && c.Status == "warn" && strings.Contains(c.Message, "unverified") {
			sawIntegWarn = true
		}
		if c.Name == "instructions_capability" && c.Status == "warn" {
			sawInstrWarn = true
		}
	}
	if !sawIntegWarn || !sawInstrWarn {
		t.Fatalf("expected runtime_integration and instructions_capability warnings in doctor: %+v", envDoc.Checks)
	}
}
