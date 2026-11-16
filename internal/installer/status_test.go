package installer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/catalog"
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
