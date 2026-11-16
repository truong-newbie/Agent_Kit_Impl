package installer

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/apperr"
	"dtkit.local/dtkit/internal/catalog"
)

func TestRemovePreservesEmptyExistingAgentsAndPersonalSkillFiles(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(root, "AGENTS.md")
	// Pre-existing empty AGENTS.md (A43).
	if err := os.WriteFile(agents, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	pkg, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, source, "project")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	if _, err := FreshInstall("project", roots, pkg, source, artifacts, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Add personal file inside DTKit skill directory (A12).
	personalNote := filepath.Join(root, ".agents", "skills", "dtkit-project-plan", "notes.txt")
	if err := os.WriteFile(personalNote, []byte("keep me\n"), 0644); err != nil {
		t.Fatal(err)
	}

	res, err := Remove("project", "engineer", roots, time.Now(), RemoveOptions{})
	if err != nil || !res.Removed {
		t.Fatalf("remove failed: %+v err=%v", res, err)
	}

	// Empty pre-existing AGENTS.md must be preserved.
	b, err := os.ReadFile(agents)
	if err != nil || len(b) != 0 {
		t.Fatalf("expected empty pre-existing AGENTS.md preserved, got %q err=%v", b, err)
	}
	// Personal file in skill directory must survive.
	noteBytes, err := os.ReadFile(personalNote)
	if err != nil || string(noteBytes) != "keep me\n" {
		t.Fatalf("personal file lost: %q err=%v", noteBytes, err)
	}
	// state.json must be removed; status must be not_installed.
	if _, err := os.Stat(filepath.Join(root, ".dtkit", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("state.json survived remove: %v", err)
	}
	st, err := Inspect("project", roots, artifacts)
	if err != nil || st.Installed {
		t.Fatalf("expected not_installed after remove, got %+v err=%v", st, err)
	}
	// Reinstall succeeds cleanly on scope.
	if _, err := FreshInstall("project", roots, pkg, source, artifacts, time.Now()); err != nil {
		t.Fatalf("reinstall failed: %v", err)
	}
}

func TestRemoveDeletesCreatedAgentsAndHandlesMissingWholeFileAndDrift(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pkg, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, source, "project")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	if _, err := FreshInstall("project", roots, pkg, source, artifacts, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Delete one managed rule file (A14: missing whole-file should forget metadata on remove).
	rulePath := filepath.Join(root, ".dtkit", "content", "engineer", "rules", "engineering.md")
	if err := os.Remove(rulePath); err != nil {
		t.Fatal(err)
	}

	// Modify one skill file -> remove must stop with Conflict before changing anything.
	skillPath := filepath.Join(root, ".agents", "skills", "dtkit-project-plan", "SKILL.md")
	origSkill, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skillPath, []byte("custom edit"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove("project", "engineer", roots, time.Now(), RemoveOptions{}); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected conflict on modified skill, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".dtkit", "state.json")); err != nil {
		t.Fatalf("state disappeared on aborted remove: %v", err)
	}

	// Restore original skill bytes, then remove succeeds and deletes DTKit-created AGENTS.md.
	if err := os.WriteFile(skillPath, origSkill, 0644); err != nil {
		t.Fatal(err)
	}
	res, err := Remove("project", "engineer", roots, time.Now(), RemoveOptions{})
	if err != nil || !res.Removed {
		t.Fatalf("remove failed: %+v err=%v", res, err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("DTKit-created AGENTS.md was not removed: %v", err)
	}
}

func TestRemoveFaultInjectionRollsBackCleanly(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("\xef\xbb\xbf# User header\r\n"), 0644); err != nil {
		t.Fatal(err)
	}
	pkg, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, source, "project")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	if _, err := FreshInstall("project", roots, pkg, source, artifacts, time.Now()); err != nil {
		t.Fatal(err)
	}
	beforeAgents, _ := os.ReadFile(agents)

	_, err = Remove("project", "engineer", roots, time.Now(), RemoveOptions{
		Hooks: &TxHooks{
			BeforeStateCommit: func() error { return errors.New("injected state delete failure") },
		},
	})
	if err == nil {
		t.Fatal("expected injected error")
	}
	afterAgents, _ := os.ReadFile(agents)
	if !bytes.Equal(beforeAgents, afterAgents) {
		t.Fatal("AGENTS.md not restored after failed remove")
	}
	st, err := Inspect("project", roots, artifacts)
	if err != nil || !st.Installed || !st.Healthy {
		t.Fatalf("expected healthy installation after rollback, got %+v err=%v", st, err)
	}
}

func TestRemoveRejectsKitIDMismatch(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pkg, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, source, "project")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	if _, err := FreshInstall("project", roots, pkg, source, artifacts, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := Remove("project", "other-kit", roots, time.Now(), RemoveOptions{}); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict for kit mismatch, got %v", err)
	}
}
