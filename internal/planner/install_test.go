package planner

import (
	"os"
	"path/filepath"
	"testing"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/catalog"
)

func TestInstallPreviewDoesNotWriteAndDetectsConflicts(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir()); if err != nil { t.Fatal(err) }
	p, source, err := catalog.Engineer(); if err != nil { t.Fatal(err) }
	artifacts, err := codex.Render(p, source, "project"); if err != nil { t.Fatal(err) }
	plan, err := InstallPreview("project", map[string]string{"project": root}, artifacts); if err != nil { t.Fatal(err) }
	if len(plan.Actions) != 15 || plan.Conflicts != 0 { t.Fatalf("unexpected plan: %+v", plan) }
	entries, err := os.ReadDir(root); if err != nil || len(entries) != 0 { t.Fatalf("preview wrote files: %v %v", entries, err) }
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("personal rules"), 0600); err != nil { t.Fatal(err) }
	plan, err = InstallPreview("project", map[string]string{"project": root}, artifacts); if err != nil { t.Fatal(err) }
	for _, action := range plan.Actions { if action.Path == "AGENTS.md" && action.Kind != "update" { t.Fatalf("unexpected instruction action: %+v", action) } }
	if err := os.WriteFile(filepath.Join(root, "AGENTS.override.md"), []byte("override"), 0600); err != nil { t.Fatal(err) }
	plan, err = InstallPreview("project", map[string]string{"project": root}, artifacts); if err != nil { t.Fatal(err) }
	if plan.Conflicts != 1 { t.Fatalf("expected override conflict: %+v", plan) }
	contents, err := os.ReadFile(filepath.Join(root, "AGENTS.md")); if err != nil || string(contents) != "personal rules" { t.Fatal("preview changed user file") }
}

func TestInstallPreviewRejectsTraversal(t *testing.T) {
	_, err := safeTarget(t.TempDir(), "../escape")
	if err == nil { t.Fatal("accepted traversal") }
}

func TestInstallPreviewBlocksExistingState(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir()); if err != nil { t.Fatal(err) }
	if err := os.Mkdir(filepath.Join(root, ".dtkit"), 0700); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(root, ".dtkit", "state.json"), []byte("{}"), 0600); err != nil { t.Fatal(err) }
	if _, err := InstallPreview("project", map[string]string{"project": root}, nil); err == nil { t.Fatal("accepted existing state") }
}

func TestInstallPreviewPreservesUnownedSkill(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir()); if err != nil { t.Fatal(err) }
	p, source, err := catalog.Engineer(); if err != nil { t.Fatal(err) }
	artifacts, err := codex.Render(p, source, "project"); if err != nil { t.Fatal(err) }
	skillDir := filepath.Join(root, ".agents", "skills", "dtkit-project-plan")
	if err := os.MkdirAll(skillDir, 0700); err != nil { t.Fatal(err) }
	skill := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skill, []byte("personal skill"), 0600); err != nil { t.Fatal(err) }
	plan, err := InstallPreview("project", map[string]string{"project": root}, artifacts); if err != nil { t.Fatal(err) }
	if plan.Conflicts != 1 { t.Fatalf("expected unowned skill conflict: %+v", plan) }
	content, err := os.ReadFile(skill); if err != nil || string(content) != "personal skill" { t.Fatal("preview modified user skill") }
}

func TestGlobalPreviewUsesSeparateCodexHome(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir()); if err != nil { t.Fatal(err) }
	codexHome, err := filepath.EvalSymlinks(t.TempDir()); if err != nil { t.Fatal(err) }
	p, source, err := catalog.Engineer(); if err != nil { t.Fatal(err) }
	artifacts, err := codex.Render(p, source, "global"); if err != nil { t.Fatal(err) }
	plan, err := InstallPreview("global", map[string]string{"user-home": home, "codex-home": codexHome}, artifacts); if err != nil { t.Fatal(err) }
	for _, action := range plan.Actions {
		if action.Path == "AGENTS.md" && action.Root != "codex-home" { t.Fatal("instruction not in codex home") }
		if action.Path != "AGENTS.md" && action.Root != "user-home" { t.Fatal("skill or resource outside user home") }
	}
	for _, root := range []string{home, codexHome} { entries, err := os.ReadDir(root); if err != nil || len(entries) != 0 { t.Fatalf("preview wrote files: %v %v", entries, err) } }
}
