package installer

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func buildSecondKit(id, version, ruleBody, reviewNote string) (catalog.Package, fstest.MapFS) {
	files := fstest.MapFS{
		"rules/security.md":           &fstest.MapFile{Data: []byte(ruleBody)},
		"skills/review/SKILL.md":      &fstest.MapFile{Data: []byte("---\nname: dtkit-claude-project-" + id + "-review\ndescription: Security code review\n---\n# Security Review\n" + reviewNote + "\n")},
		"skills/review/SKILL.md.tmpl": &fstest.MapFile{Data: []byte("---\nname: {{.SkillName}}\ndescription: Security code review\n---\n# Security Review\n" + reviewNote + "\n")},
	}
	delete(files, "skills/review/SKILL.md")
	pkg := catalog.Package{
		SchemaVersion:        1,
		ID:                   id,
		Name:                 "Security Review Kit",
		Version:              version,
		Description:          "Security audit skills",
		RuntimeTargets:       []string{"codex", "claude"},
		Scopes:               []string{"project", "global"},
		RequiresCapabilities: []string{"skills"},
		OptionalCapabilities: []string{"instructions"},
		PackageDependencies:  []string{},
		Components: []catalog.Component{
			{ID: "rule/security", Kind: "rule", Source: "rules/security.md", Requires: []string{}},
			{ID: "skill/review", Kind: "skill", Source: "skills/review", Requires: []string{"rule/security"}},
		},
	}
	return pkg, files
}

func TestMultiKitCoexistenceUpdateHookCrashAndRemove(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "repo")
	home := filepath.Join(base, "home")
	claudeHome := filepath.Join(base, "claude-home")
	exePath := filepath.Join(base, "bin", "dtkit")
	for _, d := range []string{proj, home, claudeHome, filepath.Dir(exePath)} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(exePath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	userNotes := "# Personal Claude Instructions\nDo not touch.\n"
	if err := os.WriteFile(filepath.Join(proj, "CLAUDE.md"), []byte(userNotes), 0644); err != nil {
		t.Fatal(err)
	}
	userSettings := `{"permissions":{"allow":["Read"]}}` + "\n"
	if err := os.MkdirAll(filepath.Join(proj, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".claude", "settings.json"), []byte(userSettings), 0644); err != nil {
		t.Fatal(err)
	}

	roots := map[string]string{
		"project":     proj,
		"user-home":   home,
		"claude-home": claudeHome,
	}
	globRoots := map[string]string{
		"user-home":   home,
		"claude-home": claudeHome,
	}
	now := time.Date(2026, 11, 17, 12, 10, 0, 0, time.UTC)

	engPkg, engFS, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	engArts, err := adapter.RenderForRuntime("claude", engPkg, engFS, "project")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FreshInstallWithOptions("project", roots, engPkg, engFS, engArts, now, InstallOptions{Runtime: "claude"}); err != nil {
		t.Fatalf("install engineer failed: %v", err)
	}
	if _, _, err := EnableHookForRuntime("claude", "project", "check-changed", roots, globRoots, exePath, false, now.Add(time.Second), nil); err != nil {
		t.Fatalf("enable hook failed: %v", err)
	}

	secPkgV1, secFSV1 := buildSecondKit("security-review", "0.1.0", "Audit authz and SQL boundaries.\n", "Check secrets and bounds.")
	secArtsV1, err := adapter.RenderForRuntime("claude", secPkgV1, secFSV1, "project")
	if err != nil {
		t.Fatal(err)
	}

	// Simulate crash before state commit during 2nd kit install (v2 -> v3), then recover cleanly.
	_, crashErr := FreshInstallWithOptions("project", roots, secPkgV1, secFSV1, secArtsV1, now.Add(2*time.Second), InstallOptions{
		Runtime: "claude",
		Hooks: &TxHooks{
			BeforeStateCommit: func() error { return errors.New("simulated crash") },
		},
	})
	if crashErr == nil {
		t.Fatal("expected injected crash error during 2nd kit install")
	}
	if _, err := RecoverForRuntime("claude", "project", roots, "", false, now.Add(3*time.Second)); err != nil {
		t.Fatalf("recover failed: %v", err)
	}
	reportsAfterCrash, err := InspectRuntimeAll("claude", "project", roots)
	if err != nil || len(reportsAfterCrash) != 1 || reportsAfterCrash[0].KitID != "engineer" || !reportsAfterCrash[0].Healthy {
		t.Fatalf("expected clean single-kit recovery, got err=%v reports=%+v", err, reportsAfterCrash)
	}

	// Now install security-review cleanly -> state promotes to v3 Registry.
	if _, err := FreshInstallWithOptions("project", roots, secPkgV1, secFSV1, secArtsV1, now.Add(4*time.Second), InstallOptions{Runtime: "claude"}); err != nil {
		t.Fatalf("install 2nd kit security-review failed: %v", err)
	}
	stateRaw, err := os.ReadFile(filepath.Join(proj, ".dtkit", "runtimes", "claude", "state.json"))
	if err != nil || !strings.Contains(string(stateRaw), `"schemaVersion": 3`) {
		t.Fatalf("expected v3 Registry on disk, err=%v:\n%s", err, string(stateRaw))
	}

	// Both engineer review skill AND security-review skill coexist without colliding.
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-review", "SKILL.md")); err != nil {
		t.Fatalf("missing engineer review skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-security-review-review", "SKILL.md")); err != nil {
		t.Fatalf("missing security-review skill: %v", err)
	}
	claudeMD, err := os.ReadFile(filepath.Join(proj, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Personal Claude Instructions", "<!-- dtkit:begin engineer claude project -->", "<!-- dtkit:begin security-review claude project -->"} {
		if !strings.Contains(string(claudeMD), want) {
			t.Fatalf("CLAUDE.md missing %q:\n%s", want, string(claudeMD))
		}
	}

	// Planner must reject conflicting whole-file ownership explicitly (never silently overwrite).
	dupArts := []adapter.Artifact{{
		Root:        "project",
		Path:        ".claude/skills/dtkit-claude-project-review/SKILL.md",
		Strategy:    "whole-file",
		ComponentID: "skill/review",
		Content:     []byte("hijack"),
	}}
	regLoaded, exists, err := state.LoadRegistryForRuntime(filepath.Join(proj, ".dtkit", "runtimes", "claude", "state.json"), "project", "claude")
	if err != nil || !exists {
		t.Fatalf("LoadRegistryForRuntime failed: exists=%v err=%v", exists, err)
	}
	conflictPlan, err := planner.PlanDesiredWithPeersForRuntime("claude", "project", "malicious-kit", roots, nil, regLoaded.PeerOwnedWholeFiles("malicious-kit"), dupArts)
	if err != nil || conflictPlan.Conflicts != 1 || conflictPlan.Actions[0].Reason != "artifact_owned_by_engineer" {
		t.Fatalf("expected explicit artifact_owned_by_engineer conflict, got err=%v plan=%+v", err, conflictPlan)
	}

	// Update security-review 0.1.0 -> 0.2.0 while engineer & hook remain intact.
	secPkgV2, secFSV2 := buildSecondKit("security-review", "0.2.0", "Audit authz, SSRF, and SQL boundaries.\n", "Check secrets, SSRF, and bounds.")
	secArtsV2, err := adapter.RenderForRuntime("claude", secPkgV2, secFSV2, "project")
	if err != nil {
		t.Fatal(err)
	}
	updRes, err := Update("project", roots, secPkgV2, secFSV2, secArtsV2, now.Add(5*time.Second), UpdateOptions{Runtime: "claude"})
	if err != nil || !updRes.Updated || updRes.Installation.Kit.Version != "0.2.0" {
		t.Fatalf("update security-review failed: err=%v res=%+v", err, updRes)
	}

	reports2, err := InspectRuntimeAll("claude", "project", roots)
	if err != nil || len(reports2) != 2 || !reports2[0].Healthy || !reports2[1].Healthy {
		t.Fatalf("expected 2 healthy reports after update, err=%v reports=%+v", err, reports2)
	}

	// Concurrent operation against kit A and kit B in the same (runtime, scope) serializes cleanly under lock.
	pkg3, fs3 := buildSecondKit("third-kit", "0.1.0", "Third rule.\n", "Third note.")
	arts3, _ := adapter.RenderForRuntime("claude", pkg3, fs3, "project")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, e := FreshInstallWithOptions("project", roots, pkg3, fs3, arts3, now.Add(6*time.Second), InstallOptions{Runtime: "claude"})
		errs <- e
	}()
	go func() {
		defer wg.Done()
		secPkgV3, secFSV3 := buildSecondKit("security-review", "0.3.0", "Audit authz v3.\n", "v3")
		secArtsV3, _ := adapter.RenderForRuntime("claude", secPkgV3, secFSV3, "project")
		_, e := Update("project", roots, secPkgV3, secFSV3, secArtsV3, now.Add(6*time.Second), UpdateOptions{Runtime: "claude"})
		errs <- e
	}()
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			var appErr *apperr.Error
			if !errors.As(e, &appErr) || appErr.Code != "lock_held" {
				t.Fatalf("unexpected non-lock concurrency error: %v", e)
			}
		}
	}

	// Remove engineer while security-review (and third-kit if installed) + user content + shared hooks stay intact.
	remEng, err := Remove("project", "engineer", roots, now.Add(7*time.Second), RemoveOptions{Runtime: "claude"})
	if err != nil || !remEng.Removed {
		t.Fatalf("remove engineer failed: err=%v res=%+v", err, remEng)
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-review", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("expected engineer skill removed")
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-security-review-review", "SKILL.md")); err != nil {
		t.Fatalf("expected security-review skill preserved after removing engineer: %v", err)
	}
	claudeAfterRem, _ := os.ReadFile(filepath.Join(proj, "CLAUDE.md"))
	if strings.Contains(string(claudeAfterRem), "dtkit:begin engineer") || !strings.Contains(string(claudeAfterRem), "Personal Claude Instructions") || !strings.Contains(string(claudeAfterRem), "dtkit:begin security-review") {
		t.Fatalf("unexpected CLAUDE.md after engineer removal:\n%s", string(claudeAfterRem))
	}
	hookStatuses, err := InspectHooksForRuntime("claude", "project", roots)
	if err != nil || len(hookStatuses) != 1 || hookStatuses[0].Status != "configured_pending_trust" {
		t.Fatalf("expected check-changed hook preserved while peer kit remains, got err=%v hooks=%+v", err, hookStatuses)
	}

	// Remove remaining kits -> cleans up shared hooks & state.json cleanly while preserving user CLAUDE.md & settings.json.
	allLeft, _ := InspectRuntimeAll("claude", "project", roots)
	for _, rep := range allLeft {
		if rep.Installed {
			if _, err := Remove("project", rep.KitID, roots, now.Add(8*time.Second), RemoveOptions{Runtime: "claude"}); err != nil {
				t.Fatalf("remove remaining %s failed: %v", rep.KitID, err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(proj, ".dtkit", "runtimes", "claude", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("expected state.json removed after removing all kits")
	}
	settingsAfter, err := os.ReadFile(filepath.Join(proj, ".claude", "settings.json"))
	if err != nil || !strings.Contains(string(settingsAfter), `"permissions"`) || strings.Contains(string(settingsAfter), "check-changed") {
		t.Fatalf("expected user settings preserved and hook unmerged:\n%s", string(settingsAfter))
	}
}
