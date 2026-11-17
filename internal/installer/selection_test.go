package installer

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func loadBundledEngineer(t *testing.T) (catalog.Package, fs.FS) {
	t.Helper()
	pkg, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	return pkg, source
}

func TestBundledEngineerPresetsAndPartialRendering(t *testing.T) {
	pkg, source := loadBundledEngineer(t)

	minimal, err := catalog.ResolveComponentSelection(pkg, source, "minimal", nil)
	if err != nil {
		t.Fatalf("ResolveComponentSelection(minimal) failed: %v", err)
	}
	wantMinimal := []string{
		"rule/engineering",
		"skill/ask",
		"skill/explain",
		"skill/plan",
		"template/implementation-plan",
	}
	if !reflect.DeepEqual(minimal.ResolvedComponents, wantMinimal) {
		t.Fatalf("minimal resolved = %v, want %v", minimal.ResolvedComponents, wantMinimal)
	}

	reviewOnly, err := catalog.ResolveComponentSelection(pkg, source, "review-only", nil)
	if err != nil {
		t.Fatalf("ResolveComponentSelection(review-only) failed: %v", err)
	}
	wantReview := []string{
		"rule/engineering",
		"skill/review",
		"template/review-report",
		"workflow/review",
	}
	if !reflect.DeepEqual(reviewOnly.ResolvedComponents, wantReview) {
		t.Fatalf("review-only resolved = %v, want %v", reviewOnly.ResolvedComponents, wantReview)
	}

	engFull, err := catalog.ResolveComponentSelection(pkg, source, "engineering", nil)
	if err != nil {
		t.Fatalf("ResolveComponentSelection(engineering) failed: %v", err)
	}
	if len(engFull.ResolvedComponents) != len(pkg.Components) {
		t.Fatalf("engineering preset resolved %d components, want all %d", len(engFull.ResolvedComponents), len(pkg.Components))
	}

	for _, rt := range []string{"codex", "claude"} {
		artifacts, err := adapter.RenderForRuntimeSelection(rt, pkg, source, "project", minimal.ResolvedComponents)
		if err != nil {
			t.Fatalf("RenderForRuntimeSelection(%s, minimal) failed: %v", rt, err)
		}
		if len(artifacts) != 6 {
			t.Fatalf("runtime %s minimal artifact count = %d, want 6 (instructions + 3 skills + rule + template): %+v", rt, len(artifacts), artifacts)
		}

		reviewArts, err := adapter.RenderForRuntimeSelection(rt, pkg, source, "project", reviewOnly.ResolvedComponents)
		if err != nil {
			t.Fatalf("RenderForRuntimeSelection(%s, review-only) failed: %v", rt, err)
		}
		if len(reviewArts) != 5 {
			t.Fatalf("runtime %s review-only artifact count = %d, want 5 (instructions + dt-review + rule + template + workflow): %+v", rt, len(reviewArts), reviewArts)
		}
	}
}

func TestPresetSwitchLifecycleCrashRollbackAndLegacyUpgrade(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "repo")
	home := filepath.Join(base, "home")
	claudeHome := filepath.Join(base, "claude-home")
	for _, d := range []string{proj, home, claudeHome} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(proj, "CLAUDE.md"), []byte("# User Notes\nPreserve me.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	roots := map[string]string{
		"project":     proj,
		"user-home":   home,
		"claude-home": claudeHome,
		"codex-home":  filepath.Join(home, ".codex"),
	}
	now := time.Date(2026, 11, 17, 11, 0, 0, 0, time.UTC)

	pkg, source := loadBundledEngineer(t)

	minimalSel, err := catalog.ResolveComponentSelection(pkg, source, "minimal", nil)
	if err != nil {
		t.Fatal(err)
	}
	minimalArts, err := adapter.RenderForRuntimeSelection("claude", pkg, source, "project", minimalSel.ResolvedComponents)
	if err != nil {
		t.Fatal(err)
	}

	inst, err := FreshInstallWithOptions("project", roots, pkg, source, minimalArts, now, InstallOptions{
		Runtime:    "claude",
		SourceKind: "bundled",
		Selection:  minimalSel,
	})
	if err != nil {
		t.Fatalf("FreshInstall(minimal) failed: %v", err)
	}
	if inst.Selection == nil || inst.Selection.Preset != "minimal" {
		t.Fatalf("unexpected saved selection: %+v", inst.Selection)
	}

	planSkill := filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-plan", "SKILL.md")
	reviewSkill := filepath.Join(proj, ".claude", "skills", "dtkit-claude-project-review", "SKILL.md")
	if _, err := os.Stat(planSkill); err != nil {
		t.Fatalf("expected %s after minimal install: %v", planSkill, err)
	}
	if _, err := os.Stat(reviewSkill); !os.IsNotExist(err) {
		t.Fatalf("did not expect %s after minimal install", reviewSkill)
	}

	// Switch to review-only: removes minimal-only skills, keeps shared instruction block, adds dt-review.
	reviewSel, err := catalog.ResolveComponentSelection(pkg, source, "review-only", nil)
	if err != nil {
		t.Fatal(err)
	}
	reviewArts, err := adapter.RenderForRuntimeSelection("claude", pkg, source, "project", reviewSel.ResolvedComponents)
	if err != nil {
		t.Fatal(err)
	}
	upd, err := Update("project", roots, pkg, source, reviewArts, now.Add(time.Minute), UpdateOptions{
		Runtime:    "claude",
		SourceKind: "bundled",
		Selection:  reviewSel,
	})
	if err != nil {
		t.Fatalf("Update(review-only) failed: %v", err)
	}
	if upd.Noop {
		t.Fatalf("expected non-noop update when switching presets")
	}
	if _, err := os.Stat(reviewSkill); err != nil {
		t.Fatalf("expected %s after switching to review-only: %v", reviewSkill, err)
	}
	if _, err := os.Stat(planSkill); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed when switching to review-only", planSkill)
	}
	claudeMD, err := os.ReadFile(filepath.Join(proj, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(claudeMD) == "" || !containsAll(string(claudeMD), "# User Notes", "Preserve me.", "<!-- dtkit:begin engineer claude project -->") {
		t.Fatalf("CLAUDE.md user notes or managed block damaged:\n%s", string(claudeMD))
	}

	report, err := InspectRuntime("claude", "project", roots, reviewArts)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Healthy || report.Selection == nil || report.Selection.Preset != "review-only" {
		t.Fatalf("unexpected status after preset switch: %+v", report)
	}

	// Inject crash right before state commit while switching back to minimal, then recover.
	_, err = Update("project", roots, pkg, source, minimalArts, now.Add(2*time.Minute), UpdateOptions{
		Runtime:    "claude",
		SourceKind: "bundled",
		Selection:  minimalSel,
		Hooks: &TxHooks{
			BeforeStateCommit: func() error {
				return errors.New("simulated crash before state commit")
			},
		},
	})
	if err == nil {
		t.Fatalf("expected injected crash error during preset switch update")
	}
	rec, err := RecoverForRuntime("claude", "project", roots, "", false, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("RecoverForRuntime failed: %v", err)
	}
	if rec.Outcome != "noop" {
		t.Fatalf("expected clean noop recover after automatic synchronous rollback, got %+v", rec)
	}
	if _, err := os.Stat(reviewSkill); err != nil {
		t.Fatalf("expected %s restored after rollback: %v", reviewSkill, err)
	}
	if _, err := os.Stat(planSkill); !os.IsNotExist(err) {
		t.Fatalf("expected %s removed after rollback", planSkill)
	}

	afterRec, err := InspectRuntime("claude", "project", roots, reviewArts)
	if err != nil {
		t.Fatal(err)
	}
	if !afterRec.Healthy || afterRec.Selection == nil || afterRec.Selection.Preset != "review-only" {
		t.Fatalf("unexpected status after rollback recovery: %+v", afterRec)
	}

	// Verify legacy installation without recorded selection upgrades cleanly with explicit component selection.
	legacyProj := filepath.Join(base, "legacy-repo")
	if err := os.MkdirAll(legacyProj, 0755); err != nil {
		t.Fatal(err)
	}
	legacyRoots := map[string]string{
		"project":    legacyProj,
		"user-home":  home,
		"codex-home": filepath.Join(home, ".codex"),
	}
	v1Pkg, v1FS := fixtureKit("0.3.0", "Legacy rule\n", true, true)
	v1Arts, err := adapter.RenderForRuntime("codex", v1Pkg, v1FS, "project")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FreshInstallWithOptions("project", legacyRoots, v1Pkg, v1FS, v1Arts, now, InstallOptions{Runtime: "codex"}); err != nil {
		t.Fatal(err)
	}
	compSel, err := catalog.ResolveComponentSelection(pkg, source, "", []string{"skill/plan"})
	if err != nil {
		t.Fatal(err)
	}
	compArts, err := adapter.RenderForRuntimeSelection("codex", pkg, source, "project", compSel.ResolvedComponents)
	if err != nil {
		t.Fatal(err)
	}
	updLegacy, err := Update("project", legacyRoots, pkg, source, compArts, now.Add(4*time.Minute), UpdateOptions{
		Runtime:   "codex",
		Selection: compSel,
	})
	if err != nil {
		t.Fatalf("legacy upgrade with component selection failed: %v", err)
	}
	if updLegacy.Installation.Selection == nil || !reflect.DeepEqual(updLegacy.Installation.Selection.RequestedComponents, []string{"skill/plan"}) {
		t.Fatalf("expected upgraded selection saved, got %+v", updLegacy.Installation.Selection)
	}
	_ = paths.DefaultEnvironment
}

func containsAll(haystack string, needles ...string) bool {
	for _, n := range needles {
		found := false
		for i := 0; i+len(n) <= len(haystack); i++ {
			if haystack[i:i+len(n)] == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
