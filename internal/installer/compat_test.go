package installer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func TestMainBaselineUpgradeAndGoldenLifecycleAcrossRuntimes(t *testing.T) {
	base := t.TempDir()
	proj := filepath.Join(base, "proj")
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(home, ".codex")
	geminiHome := filepath.Join(home, ".gemini")
	ocHome := filepath.Join(home, ".config", "opencode")
	for _, d := range []string{proj, home, codexHome} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Reconstruct the exact main (f81ce6e) engineer@0.1.0 project + global Codex installation
	// alongside personal AGENTS.md bytes (UTF-8 BOM + CRLF) and v2 hook registrations.
	userHeader := []byte("\xef\xbb\xbf# Personal Workspace Rules\r\nKeep commits focused.\r\n")
	if err := os.WriteFile(filepath.Join(proj, "AGENTS.md"), userHeader, 0644); err != nil {
		t.Fatal(err)
	}

	mainPkg, mainFS, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	mainPkg.Version = "0.1.0"
	mainPkg.RuntimeTargets = []string{"codex"}
	oldTime := time.Date(2026, 11, 16, 10, 43, 38, 0, time.UTC)

	oldProjArts, err := codex.Render(mainPkg, mainFS, "project")
	if err != nil {
		t.Fatal(err)
	}
	projRoots := map[string]string{"project": proj}
	oldProjState, err := FreshInstallWithOptions("project", projRoots, mainPkg, mainFS, oldProjArts, oldTime, InstallOptions{
		Runtime:    "codex",
		SourceKind: "bundled",
	})
	if err != nil {
		t.Fatalf("main baseline project setup: %v", err)
	}

	oldGlobArts, err := codex.Render(mainPkg, mainFS, "global")
	if err != nil {
		t.Fatal(err)
	}
	globRoots := map[string]string{"user-home": home, "codex-home": codexHome}
	if _, err := FreshInstallWithOptions("global", globRoots, mainPkg, mainFS, oldGlobArts, oldTime, InstallOptions{
		Runtime:    "codex",
		SourceKind: "bundled",
	}); err != nil {
		t.Fatalf("main baseline global setup: %v", err)
	}

	// Enable a v0.2 SessionStart hook on project scope before upgrading from 0.1.0 -> bundled 0.2.0.
	exePath := filepath.Join(base, "bin", "dtkit")
	if err := os.MkdirAll(filepath.Dir(exePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exePath, []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnableHook("project", "session-context", projRoots, globRoots, exePath, false, oldTime.Add(time.Minute), nil); err != nil {
		t.Fatalf("enable hook on legacy installation: %v", err)
	}

	// Upgrade both project and global to the current bundled multi-runtime engineer (0.2.0).
	curPkg, curFS, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	if curPkg.Version == "0.1.0" {
		t.Fatalf("bundled engineer must exceed main 0.1.0 version for upgrade, got %s", curPkg.Version)
	}
	newProjArts, err := codex.Render(curPkg, curFS, "project")
	if err != nil {
		t.Fatal(err)
	}
	upgradeTime := time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC)
	upProj, err := Update("project", projRoots, curPkg, curFS, newProjArts, upgradeTime, UpdateOptions{
		Runtime:    "codex",
		SourceKind: "bundled",
	})
	if err != nil || !upProj.Updated {
		t.Fatalf("project upgrade from main 0.1.0 failed: %+v err=%v", upProj, err)
	}
	if upProj.Installation.InstalledAt != oldProjState.InstalledAt {
		t.Fatalf("InstalledAt changed across upgrade: got %s want %s", upProj.Installation.InstalledAt, oldProjState.InstalledAt)
	}
	if len(upProj.Installation.Hooks) != 1 || upProj.Installation.SchemaVersion != 2 {
		t.Fatalf("hook registration not preserved across upgrade: %+v", upProj.Installation)
	}

	agentsBytes, err := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
	if err != nil || !bytes.HasPrefix(agentsBytes, userHeader) {
		t.Fatalf("personal BOM/CRLF AGENTS.md prefix not preserved after upgrade: %q err=%v", agentsBytes, err)
	}

	// Repeat update is idempotent no-op.
	noopProj, err := Update("project", projRoots, curPkg, curFS, newProjArts, upgradeTime.Add(time.Minute), UpdateOptions{
		Runtime:    "codex",
		SourceKind: "bundled",
	})
	if err != nil || !noopProj.Noop {
		t.Fatalf("expected idempotent noop after upgrade, got %+v err=%v", noopProj, err)
	}

	// Global upgrade succeeds and remains healthy.
	newGlobArts, err := codex.Render(curPkg, curFS, "global")
	if err != nil {
		t.Fatal(err)
	}
	upGlob, err := Update("global", globRoots, curPkg, curFS, newGlobArts, upgradeTime, UpdateOptions{
		Runtime:    "codex",
		SourceKind: "bundled",
	})
	if err != nil || !upGlob.Updated {
		t.Fatalf("global upgrade from main 0.1.0 failed: %+v err=%v", upGlob, err)
	}

	// Same-version tampered digest continues to fail as immutable_version with explicit hint.
	tamperedPkg := curPkg
	tamperedPkg.Version = upProj.Installation.Kit.Version
	curDigest, err := catalog.Digest(curFS)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = PlanUpdateForRuntime("codex", "project", projRoots, tamperedPkg, "f"+curDigest[1:], newProjArts, codex.AdapterRevision)
	if appErr := apperr.AsError(err); appErr == nil || appErr.Code != "immutable_version" || appErr.Hint == "" {
		t.Fatalf("expected immutable_version with actionable hint, got %v", err)
	}

	// Lifecycle checks across every supported runtime state fixture (status -> recover noop -> remove).
	runtimeCases := []struct {
		rt    string
		scope string
		roots map[string]string
	}{
		{"gemini", "global", map[string]string{"user-home": home, "gemini-home": geminiHome}},
		{"opencode", "global", map[string]string{"user-home": home, "opencode-home": ocHome}},
		{"cursor", "project", projRoots},
		{"cursor", "global", map[string]string{"user-home": home}},
	}
	for _, rc := range runtimeCases {
		arts, rErr := adapter.RenderForRuntime(rc.rt, curPkg, curFS, rc.scope)
		if rErr != nil {
			t.Fatalf("%s/%s render: %v", rc.rt, rc.scope, rErr)
		}
		if _, err := FreshInstallWithOptions(rc.scope, rc.roots, curPkg, curFS, arts, upgradeTime, InstallOptions{
			Runtime:    rc.rt,
			SourceKind: "bundled",
		}); err != nil {
			t.Fatalf("%s/%s install: %v", rc.rt, rc.scope, err)
		}
		rep, err := InspectRuntime(rc.rt, rc.scope, rc.roots, arts)
		if err != nil || !rep.Installed || !rep.Healthy {
			t.Fatalf("%s/%s status: %+v err=%v", rc.rt, rc.scope, rep, err)
		}
		rec, err := RecoverForRuntime(rc.rt, rc.scope, rc.roots, "", true, upgradeTime)
		if err != nil || rec.Outcome != "noop" {
			t.Fatalf("%s/%s recover noop: %+v err=%v", rc.rt, rc.scope, rec, err)
		}
		rm, err := Remove(rc.scope, curPkg.ID, rc.roots, upgradeTime, RemoveOptions{Runtime: rc.rt})
		if err != nil || !rm.Removed {
			t.Fatalf("%s/%s remove: %+v err=%v", rc.rt, rc.scope, rm, err)
		}
	}

	// Verify frozen historical v1 journal fixture (empty Runtime -> codex) still validates cleanly for recovery.
	legacyJournal := Journal{
		SchemaVersion: journalSchemaVer,
		ID:            "legacytx1234",
		Operation:     "install",
		Scope:         "project",
		KitID:         "engineer",
		Phase:         phaseRolledBack,
		CreatedAt:     oldTime.Format(time.RFC3339),
		UpdatedAt:     oldTime.Format(time.RFC3339),
		Actions: []JournalAction{{
			Root:     "project",
			Path:     "AGENTS.md",
			Kind:     "create",
			Strategy: "managed-block",
			Status:   "rolled_back",
		}},
	}
	rawJ, err := json.Marshal(legacyJournal)
	if err != nil {
		t.Fatal(err)
	}
	var parsedJ Journal
	if err := json.Unmarshal(rawJ, &parsedJ); err != nil {
		t.Fatal(err)
	}
	if err := validateJournal(parsedJ, "project"); err != nil {
		t.Fatalf("legacy v1 journal failed compatibility check: %v", err)
	}

	// Keep unused state/planner imports clearly exercised via types.
	var _ state.Installation = upProj.Installation
	var _ planner.InstallPlan = upProj.Plan
}
