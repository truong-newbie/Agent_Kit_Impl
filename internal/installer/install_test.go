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
	if runtime.GOOS != "windows" {
		info, err := os.Stat(agents)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0644 {
			t.Fatalf("mode changed: %v", info.Mode().Perm())
		}
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
	if stored.LastTransactionID == "" {
		t.Fatal("missing lastTransactionID in state")
	}
	if _, err := FreshInstall("project", map[string]string{"project": root}, pkg, source, artifacts, now); err == nil {
		t.Fatal("second fresh install unexpectedly succeeded")
	}
	if _, err := os.Stat(filepath.Join(root, ".dtkit", "local", "install.lock")); !os.IsNotExist(err) {
		t.Fatalf("lock was not released: %v", err)
	}
}

func TestFreshInstallFaultInjectionAndRollback(t *testing.T) {
	pkg, source, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, source, "project")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		hooks *TxHooks
	}{
		{"before_backup", &TxHooks{BeforeBackup: func() error { return errors.New("fail before backup") }}},
		{"before_prepared", &TxHooks{BeforePrepared: func() error { return errors.New("fail before prepared") }}},
		{"first_action", &TxHooks{BeforeAction: func(i int, _ TxAction) error {
			if i == 0 {
				return errors.New("fail first")
			}
			return nil
		}}},
		{"mid_action", &TxHooks{AfterAction: func(i int, _ TxAction) error {
			if i == 3 {
				return errors.New("fail mid")
			}
			return nil
		}}},
		{"before_state_commit", &TxHooks{BeforeStateCommit: func() error { return errors.New("fail state commit") }}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			agents := filepath.Join(root, "AGENTS.md")
			if err := os.WriteFile(agents, []byte("personal"), 0644); err != nil {
				t.Fatal(err)
			}
			_, err = FreshInstallWithOptions("project", map[string]string{"project": root}, pkg, source, artifacts, time.Now(), InstallOptions{Hooks: tc.hooks})
			if err == nil {
				t.Fatal("expected injected failure")
			}
			content, readErr := os.ReadFile(agents)
			if readErr != nil || string(content) != "personal" {
				t.Fatalf("instructions not restored: %q (%v)", content, readErr)
			}
			if _, statErr := os.Stat(filepath.Join(root, ".dtkit", "state.json")); !os.IsNotExist(statErr) {
				t.Fatalf("state survived failed install: %v", statErr)
			}
			for _, artifact := range artifacts {
				if artifact.Path == "AGENTS.md" {
					continue
				}
				if _, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(artifact.Path))); statErr == nil {
					t.Fatalf("artifact survived rollback: %s", artifact.Path)
				}
			}
		})
	}
}

func TestRollbackDoesNotOverwriteExternalEditorModification(t *testing.T) {
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
	var editedTarget string
	hooks := &TxHooks{
		AfterAction: func(i int, act TxAction) error {
			if i == 0 && act.Strategy == "whole-file" {
				editedTarget = filepath.Join(root, filepath.FromSlash(act.Path))
				if err := os.WriteFile(editedTarget, []byte("editor concurrent save\n"), 0644); err != nil {
					return err
				}
			}
			return nil
		},
		BeforeStateCommit: func() error {
			return errors.New("injected state failure")
		},
	}
	_, err = FreshInstallWithOptions("project", map[string]string{"project": root}, pkg, source, artifacts, time.Now(), InstallOptions{Hooks: hooks})
	if err == nil || !strings.Contains(err.Error(), "modified externally") {
		t.Fatalf("expected rollback conflict error, got %v", err)
	}
	data, readErr := os.ReadFile(editedTarget)
	if readErr != nil || string(data) != "editor concurrent save\n" {
		t.Fatalf("rollback overwrote user edit: %q (%v)", data, readErr)
	}
}
