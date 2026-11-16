package installer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestInspectReportsHealthyAndOwnedDrift(t *testing.T) {
	root := t.TempDir()
	pkg, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, source, "project")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	before, err := Inspect("project", roots, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if before.Installed || before.Healthy {
		t.Fatalf("unexpected pre-install status: %+v", before)
	}
	if _, err := FreshInstall("project", roots, pkg, source, artifacts, time.Now()); err != nil {
		t.Fatal(err)
	}
	healthy, err := Inspect("project", roots, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if !healthy.Installed || !healthy.Healthy {
		t.Fatalf("expected healthy status: %+v", healthy)
	}
	for _, artifact := range artifacts {
		if artifact.Strategy != "whole-file" {
			continue
		}
		target := filepath.Join(root, filepath.FromSlash(artifact.Path))
		if err := os.WriteFile(target, []byte("user change"), 0600); err != nil {
			t.Fatal(err)
		}
		break
	}
	drifted, err := Inspect("project", roots, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if drifted.Healthy {
		t.Fatalf("expected drift: %+v", drifted)
	}
	found := false
	for _, item := range drifted.Artifacts {
		if item.Status == "modified" {
			found = true
		}
	}
	if !found {
		t.Fatalf("drift was not reported: %+v", drifted)
	}
}

func TestInspectIgnoresUnownedInstructionChanges(t *testing.T) {
	root := t.TempDir()
	pkg, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, source, "project")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Personal notes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := FreshInstall("project", roots, pkg, source, artifacts, time.Now()); err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(root, "AGENTS.md")
	content, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agents, append([]byte("# Changed personal notes\n"), content[len("# Personal notes\n"):]...), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Inspect("project", roots, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Healthy {
		t.Fatalf("unowned instructions should not be drift: %+v", report)
	}
}

func TestInspectDetectsGlobalProfileMismatch(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	codexA := filepath.Join(base, "codex-a")
	codexB := filepath.Join(base, "codex-b")
	for _, d := range []string{home, codexA, codexB} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	pkg, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, source, "global")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FreshInstall("global", map[string]string{"user-home": home, "codex-home": codexA}, pkg, source, artifacts, time.Now()); err != nil {
		t.Fatal(err)
	}
	report, err := Inspect("global", map[string]string{"user-home": home, "codex-home": codexB}, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Installed || report.Healthy || !report.ProfileMismatch {
		t.Fatalf("expected profile mismatch, got %+v", report)
	}
}
