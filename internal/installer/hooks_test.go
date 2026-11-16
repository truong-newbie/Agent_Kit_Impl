package installer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func TestHooksEnableDisableCrossScopeAndRemoveLifecycle(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj")
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(base, "codex-home")
	exe := filepath.Join(base, "bin", "dtkit")
	for _, d := range []string{proj, home, codexHome, filepath.Dir(exe)} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(exe, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Seed pre-existing user hooks.json in project.
	hooksPath := filepath.Join(proj, ".codex", "hooks.json")
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0755); err != nil {
		t.Fatal(err)
	}
	userHooks := `{"hooks":{"SessionStart":[{"matcher":"startup","hooks":[{"type":"command","command":"echo user-hook"}]}]}}` + "\n"
	if err := os.WriteFile(hooksPath, []byte(userHooks), 0644); err != nil {
		t.Fatal(err)
	}

	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	projArtifacts, _ := codex.Render(pkg, src, "project")
	globArtifacts, _ := codex.Render(pkg, src, "global")
	projRoots := map[string]string{"project": proj}
	globRoots := map[string]string{"user-home": home, "codex-home": codexHome}

	if _, err := FreshInstall("project", projRoots, pkg, src, projArtifacts, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := FreshInstall("global", globRoots, pkg, src, globArtifacts, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Enable session-context in project (migrates state v1 -> v2, merges into existing hooks.json).
	stInfo, changed, err := EnableHook("project", "session-context", projRoots, globRoots, exe, false, time.Now(), nil)
	if err != nil || !changed || stInfo.Status != "configured_pending_trust" {
		t.Fatalf("enable hook failed: %+v changed=%v err=%v", stInfo, changed, err)
	}

	// Verify state migrated to schemaVersion 2 and preserved installedAt.
	loaded, exists, err := state.LoadFile(filepath.Join(proj, ".dtkit", "state.json"), "project")
	if err != nil || !exists || loaded.SchemaVersion != 2 || len(loaded.Hooks) != 1 || loaded.Generation != 2 {
		t.Fatalf("unexpected state v2 after hook enable: %+v err=%v", loaded, err)
	}

	// Enabling in global while project is enabled must fail with hook_scope_conflict.
	if _, _, err := EnableHook("global", "session-context", globRoots, projRoots, exe, false, time.Now(), nil); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected cross-scope conflict, got %v", err)
	}

	// Broken executable detected by InspectHooks and Inspect.
	_ = os.Remove(exe)
	insp, err := InspectHooks("project", projRoots)
	if err != nil || len(insp) != 1 || insp[0].Status != "broken_executable" {
		t.Fatalf("expected broken_executable, got %+v err=%v", insp, err)
	}
	if err := os.WriteFile(exe, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// Fault injection mid-disable rolls back hooks.json and state.json cleanly.
	_, _, err = DisableHook("project", "session-context", projRoots, false, time.Now(), &TxHooks{
		BeforeStateCommit: func() error { return errors.New("injected fail") },
	})
	if err == nil {
		t.Fatal("expected injected failure")
	}
	inspAfterFail, _ := InspectHooks("project", projRoots)
	if len(inspAfterFail) != 1 || !inspAfterFail[0].Enabled {
		t.Fatalf("hook should still be enabled after rollback: %+v", inspAfterFail)
	}

	// Removing the kit cleans up DTKit hook entry while preserving user hook!
	if _, err := Remove("project", "engineer", projRoots, time.Now(), RemoveOptions{}); err != nil {
		t.Fatalf("remove with active hook failed: %v", err)
	}
	hooksAfterRemove, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(hooksAfterRemove), "echo user-hook") || strings.Contains(string(hooksAfterRemove), "DTKit session context") {
		t.Fatalf("user hook lost or DTKit hook left behind:\n%s", string(hooksAfterRemove))
	}
}

func TestHooksUpdateDriftChmodAndAmbiguousRemoveHardening(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj")
	home := filepath.Join(base, "home")
	codexHome := filepath.Join(base, "codex-home")
	exeOld := filepath.Join(base, "bin-old", "dtkit")
	exeNew := filepath.Join(base, "bin-new", "dtkit")
	if runtime.GOOS == "windows" {
		exeOld += ".exe"
		exeNew += ".exe"
	}
	for _, d := range []string{proj, home, codexHome, filepath.Dir(exeOld), filepath.Dir(exeNew)} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, bin := range []string{exeOld, exeNew} {
		if err := os.WriteFile(bin, []byte("binary"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	projArtifacts, _ := codex.Render(pkg, src, "project")
	projRoots := map[string]string{"project": proj}
	globRoots := map[string]string{"user-home": home, "codex-home": codexHome}

	if _, err := FreshInstall("project", projRoots, pkg, src, projArtifacts, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, changed, err := EnableHook("project", "session-context", projRoots, globRoots, exeOld, false, time.Now(), nil); err != nil || !changed {
		t.Fatalf("enable hook failed: changed=%v err=%v", changed, err)
	}

	// 1. Update kit version while hook is enabled -> hooks & schemaVersion=2 must survive.
	pkgNext := pkg
	pkgNext.Version = "0.9.9"
	nextArtifacts, _ := codex.Render(pkgNext, src, "project")
	res, err := Update("project", projRoots, pkgNext, src, nextArtifacts, time.Now(), UpdateOptions{})
	if err != nil {
		t.Fatalf("update with active hook failed: %v", err)
	}
	if !res.Updated {
		t.Fatal("expected version update to report updated=true")
	}
	loaded, exists, err := state.LoadFile(filepath.Join(proj, ".dtkit", "state.json"), "project")
	if err != nil || !exists || loaded.SchemaVersion != 2 || len(loaded.Hooks) != 1 {
		t.Fatalf("hook registration or schemaVersion lost on update: %+v err=%v", loaded, err)
	}

	// 2. POSIX 0644 non-executable binary triggers broken_executable.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(exeOld, 0644); err != nil {
			t.Fatal(err)
		}
		insp, err := InspectHooks("project", projRoots)
		if err != nil || len(insp) != 1 || insp[0].Status != "broken_executable" {
			t.Fatalf("expected broken_executable on chmod 0644, got %+v err=%v", insp, err)
		}
		if err := os.Chmod(exeOld, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// 3. Binary moved / drift -> re-enabling without disable fails with actionable hint, then disable -> enable works.
	_, _, driftErr := EnableHook("project", "session-context", projRoots, globRoots, exeNew, false, time.Now(), nil)
	if apperr.ExitCodeOf(driftErr) != apperr.ExitConflict {
		t.Fatalf("expected conflict on moved binary enable, got %v", driftErr)
	}
	var appDrift *apperr.Error
	if !errors.As(driftErr, &appDrift) || !strings.Contains(appDrift.Hint, "disable") {
		t.Fatalf("expected actionable disable hint on drifted enable, got %+v", driftErr)
	}
	if _, changed, err := DisableHook("project", "session-context", projRoots, false, time.Now(), nil); err != nil || !changed {
		t.Fatalf("disable hook failed: changed=%v err=%v", changed, err)
	}
	if _, changed, err := EnableHook("project", "session-context", projRoots, globRoots, exeNew, false, time.Now(), nil); err != nil || !changed {
		t.Fatalf("re-enable with moved binary failed: changed=%v err=%v", changed, err)
	}

	// 4. Ambiguous duplicate hook entries in hooks.json cause Remove to fail cleanly without deleting hooks.json.
	hooksPath := filepath.Join(proj, ".codex", "hooks.json")
	rawHooks, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(rawHooks, &doc); err != nil {
		t.Fatal(err)
	}
	hMap := doc["hooks"].(map[string]any)
	arr := hMap["SessionStart"].([]any)
	hMap["SessionStart"] = append(arr, arr[0])
	dupRaw, _ := json.MarshalIndent(doc, "", "  ")
	if err := os.WriteFile(hooksPath, dupRaw, 0644); err != nil {
		t.Fatal(err)
	}

	_, _, disableErr := DisableHook("project", "session-context", projRoots, false, time.Now(), nil)
	var appDisable *apperr.Error
	if apperr.ExitCodeOf(disableErr) != apperr.ExitConflict || !errors.As(disableErr, &appDisable) || appDisable.Code != "hook_entry_ambiguous" {
		t.Fatalf("expected hook_entry_ambiguous conflict on disable, got %v", disableErr)
	}
	if _, err := Remove("project", "engineer", projRoots, time.Now(), RemoveOptions{}); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected conflict on remove with ambiguous hooks, got %v", err)
	}
	afterHooks, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("hooks.json should remain preserved after ambiguous conflict: %v", err)
	}
	if string(afterHooks) != string(dupRaw) {
		t.Fatal("hooks.json was modified despite ambiguous hook conflict")
	}
}
