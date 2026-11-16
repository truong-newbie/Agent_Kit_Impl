package installer

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/catalog"
	"dtkit.local/dtkit/internal/state"
)

func TestFreshInstallPreservesInstructionsAndWritesState(t *testing.T) {
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
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("# Personal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 16, 1, 2, 3, 0, time.UTC)
	installed, err := FreshInstall("project", map[string]string{"project": root}, pkg, source, artifacts, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := installed.Validate(artifacts); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), "# Personal\n") || !strings.Contains(string(content), "dtkit:begin engineer project") {
		t.Fatalf("unexpected AGENTS.md: %s", content)
	}
	info, err := os.Stat(agents)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatalf("mode changed: %v", info.Mode().Perm())
	}
	stateBytes, err := os.ReadFile(filepath.Join(root, ".dtkit", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored state.Installation
	if err := json.Unmarshal(stateBytes, &stored); err != nil {
		t.Fatal(err)
	}
	if err := stored.Validate(artifacts); err != nil {
		t.Fatal(err)
	}
	if _, err := FreshInstall("project", map[string]string{"project": root}, pkg, source, artifacts, now); err == nil {
		t.Fatal("second fresh install unexpectedly succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, ".dtkit", "local", "install.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock was not released: %v", err)
	}
}

func TestFreshInstallRollsBackAppliedArtifacts(t *testing.T) {
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
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("personal"), 0644); err != nil {
		t.Fatal(err)
	}
	originalWriter := stateWriter
	stateWriter = func(string, []byte, os.FileMode) error { return errors.New("injected failure") }
	t.Cleanup(func() { stateWriter = originalWriter })
	if _, err := FreshInstall("project", map[string]string{"project": root}, pkg, source, artifacts, time.Now()); err == nil {
		t.Fatal("expected injected failure")
	}
	content, err := os.ReadFile(agents)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "personal" {
		t.Fatalf("instructions not restored: %q", content)
	}
	if _, err := os.Stat(filepath.Join(root, ".dtkit", "state.json")); !os.IsNotExist(err) {
		t.Fatalf("state survived failed install: %v", err)
	}
	for _, artifact := range artifacts {
		if artifact.Path == "AGENTS.md" {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(artifact.Path))); err == nil {
			t.Fatalf("artifact survived rollback: %s", artifact.Path)
		}
	}
}
