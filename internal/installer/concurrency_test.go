package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func TestCrossRuntimeBarrierInstallUpdateRemoveAndRecoverSafety(t *testing.T) {
	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	pkgV2 := pkg
	pkgV2.Version = "9.9.9"
	now := time.Date(2026, 11, 16, 14, 0, 0, 0, time.UTC)

	codexProj, err := adapter.RenderForRuntime("codex", pkg, src, "project")
	if err != nil {
		t.Fatal(err)
	}
	opencodeProj, err := adapter.RenderForRuntime("opencode", pkg, src, "project")
	if err != nil {
		t.Fatal(err)
	}
	cursorProj, err := adapter.RenderForRuntime("cursor", pkg, src, "project")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Concurrent install/install on shared <project>/AGENTS.md using a barrier inside Codex's AGENTS.md write.
	t.Run("install_install_shared_agents_no_lost_block", func(t *testing.T) {
		proj, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		agentsPath := filepath.Join(proj, "AGENTS.md")
		if err := os.WriteFile(agentsPath, []byte("# Personal Header\nkeep line\n"), 0644); err != nil {
			t.Fatal(err)
		}
		roots := map[string]string{"project": proj}

		codexEnteredWrite := make(chan struct{})
		releaseCodex := make(chan struct{})
		var codexOnce sync.Once

		codexHooks := &TxHooks{
			BeforeAction: func(_ int, act TxAction) error {
				if act.Path == "AGENTS.md" {
					codexOnce.Do(func() { close(codexEnteredWrite) })
					<-releaseCodex
				}
				return nil
			},
		}

		var wg sync.WaitGroup
		var codexErr, openErr error
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, codexErr = FreshInstallWithOptions("project", roots, pkg, src, codexProj, now, InstallOptions{
				Runtime:    "codex",
				SourceKind: "bundled",
				Hooks:      codexHooks,
			})
		}()

		select {
		case <-codexEnteredWrite:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for codex install barrier; codexErr=%v", codexErr)
		}
		// While Codex holds shared target lock for AGENTS.md, OpenCode tries concurrent install.
		_, openErr = FreshInstallWithOptions("project", roots, pkg, src, opencodeProj, now, InstallOptions{
			Runtime:    "opencode",
			SourceKind: "bundled",
		})
		close(releaseCodex)
		wg.Wait()

		if codexErr != nil {
			t.Fatalf("codex install failed unexpectedly: %v", codexErr)
		}
		if appErr := apperr.AsError(openErr); appErr == nil || appErr.Code != "lock_held" {
			t.Fatalf("expected deterministic lock_held conflict on concurrent opencode install, got %v", openErr)
		}
		// OpenCode must NOT have left partial skills or committed healthy state.
		if _, err := os.Stat(filepath.Join(proj, ".opencode/skills/dtkit-opencode-project-plan/SKILL.md")); !os.IsNotExist(err) {
			t.Fatalf("opencode left partial skill file behind after conflict: err=%v", err)
		}
		if _, exists, _ := state.LoadFileForRuntime(filepath.Join(proj, ".dtkit/runtimes/opencode/state.json"), "project", "opencode"); exists {
			t.Fatalf("opencode committed state despite lock conflict")
		}
		rawAgents, err := os.ReadFile(agentsPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(rawAgents, []byte("# Personal Header")) || !bytes.Contains(rawAgents, []byte("<!-- dtkit:begin engineer project -->")) {
			t.Fatalf("AGENTS.md missing user header or codex block: %s", string(rawAgents))
		}
		if entries, _ := os.ReadDir(filepath.Join(proj, ".dtkit/locks")); len(entries) != 0 {
			t.Fatalf("expected zero leftover shared locks after release, found %d", len(entries))
		}
	})

	// 2. Sequential coexistence succeeds, then concurrent update/update with barrier before ExecuteTx never loses a block.
	t.Run("update_update_pre_lock_race_and_nonoverlapping_cursor", func(t *testing.T) {
		proj, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		roots := map[string]string{"project": proj}
		if _, err := FreshInstallWithOptions("project", roots, pkg, src, codexProj, now, InstallOptions{Runtime: "codex", SourceKind: "bundled"}); err != nil {
			t.Fatal(err)
		}
		if _, err := FreshInstallWithOptions("project", roots, pkg, src, opencodeProj, now, InstallOptions{Runtime: "opencode", SourceKind: "bundled"}); err != nil {
			t.Fatal(err)
		}

		// First prove non-overlapping runtime (Cursor) installs cleanly EVEN WHILE Codex holds lock on AGENTS.md!
		codexHold := make(chan struct{})
		releaseHold := make(chan struct{})
		var wg sync.WaitGroup
		var codexUpErr error
		codexUpProj, _ := adapter.RenderForRuntime("codex", pkgV2, src, "project")
		for i := range codexUpProj {
			if codexUpProj[i].Path == "AGENTS.md" {
				codexUpProj[i].Content = bytes.ReplaceAll(codexUpProj[i].Content, []byte("DTKit engineering guidance"), []byte("DTKit engineering guidance v0.3"))
			}
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, codexUpErr = Update("project", roots, pkgV2, src, codexUpProj, now.Add(time.Minute), UpdateOptions{
				Runtime: "codex",
				Hooks: &TxHooks{
					BeforeAction: func(_ int, act TxAction) error {
						if act.Path == "AGENTS.md" {
							close(codexHold)
							<-releaseHold
						}
						return nil
					},
				},
			})
		}()
		select {
		case <-codexHold:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for codex update BeforeAction(AGENTS.md) barrier; codexUpErr=%v", codexUpErr)
		}
		// Cursor touches only .cursor/* files, NOT AGENTS.md, so it proceeds without conflict!
		if _, curErr := FreshInstallWithOptions("project", roots, pkg, src, cursorProj, now, InstallOptions{Runtime: "cursor", SourceKind: "bundled"}); curErr != nil {
			close(releaseHold)
			wg.Wait()
			t.Fatalf("non-overlapping cursor install blocked unnecessarily while codex held AGENTS.md lock: %v", curErr)
		}
		close(releaseHold)
		wg.Wait()
		if codexUpErr != nil {
			t.Fatalf("codex update failed: %v", codexUpErr)
		}

		// Now simulate pre-lock Plan race where OpenCode attempts Update using stale AGENTS.md snapshot while Codex already updated AGENTS.md.
		rawBefore, _ := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
		opState, _, _ := state.LoadFileForRuntime(filepath.Join(proj, ".dtkit/runtimes/opencode/state.json"), "project", "opencode")
		_, staleErr := ExecuteTx(TxRequest{
			Runtime:   "opencode",
			Operation: "update",
			Scope:     "project",
			KitID:     pkgV2.ID,
			Roots:     roots,
			Actions: []TxAction{{
				Root:     "project",
				Path:     "AGENTS.md",
				Kind:     "update",
				Strategy: "managed-block",
				Existed:  true,
				Before:   []byte("stale before bytes"),
				After:    []byte("corrupted overwrite"),
				Mode:     0644,
			}},
			PrevState: opState,
			NextState: opState,
			Now:       now.Add(2 * time.Minute),
		})
		if appErr := apperr.AsError(staleErr); appErr == nil || appErr.Code != "target_changed" {
			t.Fatalf("expected target_changed conflict on stale concurrent read, got %v", staleErr)
		}
		rawAfter, _ := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
		if !bytes.Equal(rawBefore, rawAfter) {
			t.Fatalf("AGENTS.md mutated on failed stale update")
		}
		if !bytes.Contains(rawAfter, []byte("<!-- dtkit:begin engineer project -->")) || !bytes.Contains(rawAfter, []byte("<!-- dtkit:begin engineer opencode project -->")) {
			t.Fatalf("lost runtime block in AGENTS.md: %s", string(rawAfter))
		}
	})

	// 3. Concurrent remove (Codex) + update (OpenCode) preserves surviving runtime block cleanly.
	t.Run("remove_update_barrier_preserves_surviving_block", func(t *testing.T) {
		proj, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		roots := map[string]string{"project": proj}
		if _, err := FreshInstallWithOptions("project", roots, pkg, src, codexProj, now, InstallOptions{Runtime: "codex", SourceKind: "bundled"}); err != nil {
			t.Fatal(err)
		}
		if _, err := FreshInstallWithOptions("project", roots, pkg, src, opencodeProj, now, InstallOptions{Runtime: "opencode", SourceKind: "bundled"}); err != nil {
			t.Fatal(err)
		}

		openHold := make(chan struct{})
		openRelease := make(chan struct{})
		opencodeV2, _ := adapter.RenderForRuntime("opencode", pkgV2, src, "project")
		for i := range opencodeV2 {
			if opencodeV2[i].Path == "AGENTS.md" {
				opencodeV2[i].Content = bytes.ReplaceAll(opencodeV2[i].Content, []byte("DTKit engineering guidance"), []byte("DTKit engineering guidance v0.3"))
			}
		}
		var wg sync.WaitGroup
		var openUpErr error
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, openUpErr = Update("project", roots, pkgV2, src, opencodeV2, now.Add(time.Minute), UpdateOptions{
				Runtime: "opencode",
				Hooks: &TxHooks{
					BeforeAction: func(_ int, act TxAction) error {
						if act.Path == "AGENTS.md" {
							close(openHold)
							<-openRelease
						}
						return nil
					},
				},
			})
		}()
		select {
		case <-openHold:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for opencode update BeforeAction(AGENTS.md) barrier; openUpErr=%v", openUpErr)
		}
		_, rmErr := Remove("project", pkg.ID, roots, now.Add(time.Minute), RemoveOptions{Runtime: "codex"})
		close(openRelease)
		wg.Wait()
		if openUpErr != nil {
			t.Fatalf("opencode update failed: %v", openUpErr)
		}
		if appErr := apperr.AsError(rmErr); appErr == nil || appErr.Code != "lock_held" {
			t.Fatalf("expected lock_held conflict on concurrent remove, got %v", rmErr)
		}
		// Subsequent sequential remove succeeds without losing OpenCode's block!
		if _, err := Remove("project", pkg.ID, roots, now.Add(2*time.Minute), RemoveOptions{Runtime: "codex"}); err != nil {
			t.Fatalf("sequential codex remove after opencode update failed: %v", err)
		}
		rawAgents, _ := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
		if bytes.Contains(rawAgents, []byte("<!-- dtkit:begin engineer project -->")) {
			t.Fatalf("codex block still present after remove")
		}
		if !bytes.Contains(rawAgents, []byte("<!-- dtkit:begin engineer opencode project -->")) {
			t.Fatalf("opencode block was lost during concurrent remove: %s", string(rawAgents))
		}
	})

	// 4. Crashed Codex transaction + concurrent/subsequent OpenCode write -> recover reports conflict without restoring before-image over OpenCode!
	t.Run("recover_refuses_to_overwrite_concurrent_runtime_edit", func(t *testing.T) {
		proj, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		agentsPath := filepath.Join(proj, "AGENTS.md")
		if err := os.WriteFile(agentsPath, []byte("# User\n"), 0644); err != nil {
			t.Fatal(err)
		}
		roots := map[string]string{"project": proj}

		// Simulate crash right after writing AGENTS.md (before Codex state commit) by manually staging pending journal & releasing locks,
		// simulating SIGKILL after partial write.
		beforeAgents, _ := os.ReadFile(agentsPath)
		codexBlock := []byte("<!-- dtkit:begin engineer project -->\n# Codex v1\n<!-- dtkit:end engineer project -->\n")
		afterCodex, _, bErr := planner.AppendBlockForRuntime("codex", pkg.ID, beforeAgents, true, codexBlock, "project")
		if bErr != nil {
			t.Fatal(bErr)
		}
		if err := os.WriteFile(agentsPath, afterCodex, 0644); err != nil {
			t.Fatal(err)
		}
		txDir := filepath.Join(proj, ".dtkit/local/transactions/crashedtx01")
		if err := os.MkdirAll(txDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(txDir, "before-000.bin"), beforeAgents, 0600); err != nil {
			t.Fatal(err)
		}
		j := Journal{
			SchemaVersion: journalSchemaVer,
			ID:            "crashedtx01",
			Operation:     "install",
			Scope:         "project",
			Runtime:       "codex",
			KitID:         pkg.ID,
			Phase:         phaseApplying,
			CreatedAt:     now.UTC().Format(time.RFC3339),
			UpdatedAt:     now.UTC().Format(time.RFC3339),
			Actions: []JournalAction{{
				Root:         "project",
				Path:         "AGENTS.md",
				Kind:         "update",
				Strategy:     "managed-block",
				Existed:      true,
				BeforeDigest: contentHash(beforeAgents),
				AfterDigest:  contentHash(afterCodex),
				Backup:       "before-000.bin",
				Mode:         0644,
				Status:       "applied",
			}},
		}
		if err := writeJSON(filepath.Join(txDir, "journal.json"), j, 0600); err != nil {
			t.Fatal(err)
		}
		// Write dead PID lock files for crashed transaction.
		deadMeta, _ := json.Marshal(lockMetadata{Token: "tok", PID: 9999999, CreatedAt: now.UTC().Format(time.RFC3339), Operation: "install", TransactionID: "crashedtx01"})
		if err := os.WriteFile(filepath.Join(proj, ".dtkit/local/install.lock"), append(deadMeta, '\n'), 0600); err != nil {
			t.Fatal(err)
		}

		// OpenCode updates AGENTS.md (adds OpenCode block after crashed Codex block).
		if _, err := FreshInstallWithOptions("project", roots, pkg, src, opencodeProj, now.Add(time.Minute), InstallOptions{Runtime: "opencode", SourceKind: "bundled"}); err != nil {
			t.Fatalf("opencode install alongside crashed codex failed: %v", err)
		}
		withOpenCode, _ := os.ReadFile(agentsPath)

		// Now Codex recover runs: MUST detect conflict and preserve OpenCode's block + journal evidence!
		rep, recErr := RecoverForRuntime("codex", "project", roots, "", false, now.Add(2*time.Minute))
		if appErr := apperr.AsError(recErr); appErr == nil || appErr.Code != "recovery_conflict" || rep.Outcome != phaseConflict {
			t.Fatalf("expected recovery_conflict protecting opencode edits, got rep=%+v err=%v", rep, recErr)
		}
		afterRecover, _ := os.ReadFile(agentsPath)
		if !bytes.Equal(withOpenCode, afterRecover) {
			t.Fatalf("recover overwrote AGENTS.md despite concurrent runtime block!\nbefore=%s\nafter=%s", string(withOpenCode), string(afterRecover))
		}
		if _, err := os.Stat(filepath.Join(txDir, "before-000.bin")); err != nil {
			t.Fatalf("journal evidence backup deleted on conflict: %v", err)
		}
	})
}

func TestStrictStateSnapshotRevalidationUnderLock(t *testing.T) {
	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	proj, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": proj}
	now := time.Date(2026, 11, 16, 14, 0, 0, 0, time.UTC)

	codexProj, _ := adapter.RenderForRuntime("codex", pkg, src, "project")
	inst, err := FreshInstallWithOptions("project", roots, pkg, src, codexProj, now, InstallOptions{Runtime: "codex", SourceKind: "bundled"})
	if err != nil {
		t.Fatal(err)
	}

	// Mutate state.json between preview and lock acquisition while keeping Generation and Kit.Digest identical.
	modified := inst
	modified.UpdatedAt = "2099-01-01T00:00:00Z"
	modified.Artifacts = append([]state.Artifact(nil), inst.Artifacts...)
	if len(modified.Artifacts) > 0 {
		modified.Artifacts[0].Digest = strings.Repeat("e", 64)
	}
	if err := writeJSON(filepath.Join(proj, ".dtkit/state.json"), modified, 0600); err != nil {
		t.Fatal(err)
	}
	nextState := inst
	nextState.Generation = inst.Generation + 1
	_, upErr := ExecuteTx(TxRequest{
		Runtime:   "codex",
		Operation: "update",
		Scope:     "project",
		KitID:     pkg.ID,
		Roots:     roots,
		PrevState: &inst,
		NextState: &nextState,
		Now:       now.Add(time.Minute),
	})
	if appErr := apperr.AsError(upErr); appErr == nil || appErr.Code != "state_changed" {
		t.Fatalf("expected strict state_changed rejection when artifact/metadata mutated under same generation+kit digest, got %v", upErr)
	}
}

func TestAliasedGlobalRootsCaseSensitivityAndLockPolicies(t *testing.T) {
	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	sharedCfgDir := filepath.Join(home, "shared-cfg")
	if err := os.MkdirAll(sharedCfgDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Alias 1: explicit clean path; Alias 2: path via "./dummy/../shared-cfg" (+ mixed case on Windows).
	codexHomeAlias := filepath.Join(home, "sub", "..", "shared-cfg")
	openHomeAlias := sharedCfgDir
	if runtime.GOOS == "windows" {
		openHomeAlias = filepath.Join(home, "SHARED-CFG")
	}

	codexRoots := map[string]string{"user-home": home, "codex-home": codexHomeAlias}
	openRoots := map[string]string{"user-home": home, "opencode-home": openHomeAlias}

	now := time.Date(2026, 11, 16, 14, 0, 0, 0, time.UTC)
	codexGlob, _ := adapter.RenderForRuntime("codex", pkg, src, "global")
	openGlob, _ := adapter.RenderForRuntime("opencode", pkg, src, "global")

	// Verify both aliases acquire the exact same shared target lock for shared AGENTS.md!
	hold := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	var codexErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, codexErr = FreshInstallWithOptions("global", codexRoots, pkg, src, codexGlob, now, InstallOptions{
			Runtime: "codex",
			Hooks: &TxHooks{
				BeforeAction: func(_ int, act TxAction) error {
					if act.Path == "AGENTS.md" {
						close(hold)
						<-release
					}
					return nil
				},
			},
		})
	}()
	select {
	case <-hold:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for aliased global lock barrier; codexErr=%v", codexErr)
	}
	_, openErr := FreshInstallWithOptions("global", openRoots, pkg, src, openGlob, now, InstallOptions{Runtime: "opencode"})
	close(release)
	wg.Wait()
	if codexErr != nil {
		t.Fatalf("codex global install failed: %v", codexErr)
	}
	if appErr := apperr.AsError(openErr); appErr == nil || appErr.Code != "lock_held" {
		t.Fatalf("expected aliased global directories to share target lock (lock_held), got %v", openErr)
	}

	// Lock age must NEVER cause recovery to delete a live PID lock or unknown/corrupt lock!
	proj := filepath.Join(base, "lock-age-proj")
	if err := os.MkdirAll(filepath.Join(proj, ".dtkit/local"), 0755); err != nil {
		t.Fatal(err)
	}
	projRoots := map[string]string{"project": proj}
	oldLiveLock, _ := json.Marshal(lockMetadata{
		Token:     "live-tok",
		PID:       os.Getpid(), // current live process (could also represent PID reuse)
		CreatedAt: "2010-01-01T00:00:00Z",
		Operation: "install",
	})
	lockPath := filepath.Join(proj, ".dtkit/local/install.lock")
	if err := os.WriteFile(lockPath, append(oldLiveLock, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	tenYearsAgo := time.Now().Add(-10 * 365 * 24 * time.Hour)
	_ = os.Chtimes(lockPath, tenYearsAgo, tenYearsAgo)

	_, liveLockErr := RecoverForRuntime("codex", "project", projRoots, "", false, now)
	if appErr := apperr.AsError(liveLockErr); appErr == nil || appErr.Code != "active_or_uncertain_lock" {
		t.Fatalf("expected live old lock to be protected regardless of age, got %v", liveLockErr)
	}

	// Corrupt lock is also protected (`active_or_uncertain_lock`).
	if err := os.WriteFile(lockPath, []byte("{corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	_, corruptLockErr := RecoverForRuntime("codex", "project", projRoots, "", false, now)
	if appErr := apperr.AsError(corruptLockErr); appErr == nil || appErr.Code != "active_or_uncertain_lock" {
		t.Fatalf("expected corrupt lock to be protected, got %v", corruptLockErr)
	}

	// Dead PID runtime lock + owned dead shared lock ARE cleaned safely by recover!
	deadLockMeta := lockMetadata{Token: "dead-tok", PID: 9999999, CreatedAt: now.UTC().Format(time.RFC3339), Operation: "install", TransactionID: "deadtx9999"}
	deadBytes, _ := json.Marshal(deadLockMeta)
	if err := os.WriteFile(lockPath, append(deadBytes, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	deadSharedLock := filepath.Join(proj, ".dtkit/locks/deadshared.lock")
	if err := os.MkdirAll(filepath.Dir(deadSharedLock), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(deadSharedLock, append(deadBytes, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	rep, err := RecoverForRuntime("codex", "project", projRoots, "", false, now)
	if err != nil || rep.Outcome != phaseRolledBack {
		t.Fatalf("expected recover to clear dead runtime + owned shared lock, got %+v err=%v", rep, err)
	}
	if _, err := os.Stat(deadSharedLock); !os.IsNotExist(err) {
		t.Fatalf("expected dead owned shared lock removed, got err=%v", err)
	}
}

func TestMultiProcessBarrierInstallRaceOnSharedAgentsMD(t *testing.T) {
	if role := os.Getenv("DTKIT_N04_CHILD_RUNTIME"); role != "" {
		proj := os.Getenv("DTKIT_N04_PROJ")
		readyFile := os.Getenv("DTKIT_N04_READY")
		goFile := os.Getenv("DTKIT_N04_GO")
		pkg, src, err := catalog.Engineer()
		if err != nil {
			t.Fatal(err)
		}
		arts, err := adapter.RenderForRuntime(role, pkg, src, "project")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(readyFile, []byte("ready"), 0600); err != nil {
			t.Fatal(err)
		}
		for {
			if _, err := os.Stat(goFile); err == nil {
				break
			}
			time.Sleep(2 * time.Millisecond)
		}
		_, err = FreshInstallWithOptions("project", map[string]string{"project": proj}, pkg, src, arts, time.Now().UTC(), InstallOptions{
			Runtime:    role,
			SourceKind: "bundled",
		})
		if err != nil {
			// Signal clean structured conflict exit code for parent verification.
			if appErr := apperr.AsError(err); appErr != nil && (appErr.Code == "lock_held" || appErr.Code == "target_changed" || appErr.Code == "concurrent_modification") {
				fmt.Fprintf(os.Stderr, "EXPECTED_CONFLICT:%s:%s\n", role, appErr.Code)
				os.Exit(42)
			}
			t.Fatalf("unexpected child %s install error: %v", role, err)
		}
		return
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj")
	if err := os.MkdirAll(proj, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, "AGENTS.md"), []byte("# User Notes\n"), 0644); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	goFile := filepath.Join(base, "go.signal")
	readyCodex := filepath.Join(base, "ready-codex")
	readyOpen := filepath.Join(base, "ready-open")

	c1 := exec.Command(exe, "-test.run=^TestMultiProcessBarrierInstallRaceOnSharedAgentsMD$")
	c1.Env = append(os.Environ(), "DTKIT_N04_CHILD_RUNTIME=codex", "DTKIT_N04_PROJ="+proj, "DTKIT_N04_READY="+readyCodex, "DTKIT_N04_GO="+goFile)
	c2 := exec.Command(exe, "-test.run=^TestMultiProcessBarrierInstallRaceOnSharedAgentsMD$")
	c2.Env = append(os.Environ(), "DTKIT_N04_CHILD_RUNTIME=opencode", "DTKIT_N04_PROJ="+proj, "DTKIT_N04_READY="+readyOpen, "DTKIT_N04_GO="+goFile)

	if err := c1.Start(); err != nil {
		t.Fatal(err)
	}
	if err := c2.Start(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, e1 := os.Stat(readyCodex)
		_, e2 := os.Stat(readyOpen)
		if e1 == nil && e2 == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := os.WriteFile(goFile, []byte("go"), 0600); err != nil {
		t.Fatal(err)
	}

	err1 := c1.Wait()
	err2 := c2.Wait()

	// Must have deterministic outcome: either 1 winner + 1 expected conflict (exit 42), OR both clean sequential if OS scheduled serially?
	// Wait: without losing either block! Check both outcomes carefully.
	winners := 0
	for _, r := range []struct {
		rt  string
		err error
	}{{"codex", err1}, {"opencode", err2}} {
		if r.err == nil {
			winners++
		} else {
			if ee, ok := r.err.(*exec.ExitError); !ok || ee.ExitCode() != 42 {
				t.Fatalf("child %s failed with unexpected error: %v", r.rt, r.err)
			}
		}
	}
	if winners < 1 {
		t.Fatalf("expected at least 1 deterministic winner, got 0")
	}
	rawAgents, err := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(rawAgents, []byte("# User Notes")) {
		t.Fatalf("user content lost in AGENTS.md")
	}
	if err1 == nil && !bytes.Contains(rawAgents, []byte("<!-- dtkit:begin engineer project -->")) {
		t.Fatalf("codex exited 0 but its AGENTS.md block is missing! contents:\n%s", string(rawAgents))
	}
	if err2 == nil && !bytes.Contains(rawAgents, []byte("<!-- dtkit:begin engineer opencode project -->")) {
		t.Fatalf("opencode exited 0 but its AGENTS.md block is missing! contents:\n%s", string(rawAgents))
	}
}
