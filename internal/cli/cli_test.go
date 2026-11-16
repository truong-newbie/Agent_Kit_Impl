package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListAndInfo(t *testing.T) {
	for _, args := range [][]string{{"list"}, {"info", "engineer"}} {
		var out, stderr bytes.Buffer
		if err := Run(args, &out, &stderr); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(out.String(), "engineer") {
			t.Fatalf("missing engineer: %s", out.String())
		}
	}
}

func TestInstallDryRunIsReadOnly(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := Run([]string{"install", "engineer", "--project", "--path", root, "--dry-run", "--json"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"dryRun":true`) || !strings.Contains(out.String(), "dtkit-project-plan") {
		t.Fatalf("unexpected preview: %s", out.String())
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("preview wrote files: %v %v", entries, err)
	}
}

func TestUnsupportedInstallDoesNotClaimSuccess(t *testing.T) {
	var out, stderr bytes.Buffer
	if err := Run([]string{"install", "engineer", "--project"}, &out, &stderr); err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}

func TestFreshProjectInstall(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("personal\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := Run([]string{"install", "engineer", "--project", "--path", root, "--yes"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Installed engineer project") {
		t.Fatalf("unexpected output: %s", out.String())
	}
	content, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(content), "personal\n") || !strings.Contains(string(content), "dtkit:begin") {
		t.Fatalf("personal instructions were not preserved: %s", content)
	}
	if _, err := os.Stat(filepath.Join(root, ".dtkit", "state.json")); err != nil {
		t.Fatal(err)
	}
}
