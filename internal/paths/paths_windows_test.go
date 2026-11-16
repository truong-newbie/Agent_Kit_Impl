//go:build windows

package paths

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func createWindowsJunction(t *testing.T, linkPath, targetDir string) {
	t.Helper()
	cmd := exec.Command("cmd", "/c", "mklink", "/J", linkPath, targetDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mklink /J %s -> %s failed: %v\n%s", linkPath, targetDir, err, out)
	}
}

func TestResolveTargetRejectsNTFSJunctionRootAndChild(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	realDir := filepath.Join(base, "real")
	otherDir := filepath.Join(base, "other")
	siblingDir := filepath.Join(realDir, ".codex")
	for _, d := range []string{realDir, otherDir, siblingDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// 1. Junction acting as workspace root must be detected and rejected.
	junctionRoot := filepath.Join(base, "junction-root")
	createWindowsJunction(t, junctionRoot, realDir)

	info, err := os.Lstat(junctionRoot)
	if err != nil || !IsReparseOrSymlink(info) {
		t.Fatalf("expected junction root to be flagged as reparse point: info=%v err=%v", info, err)
	}
	if _, err := ResolveTarget(junctionRoot, "AGENTS.md"); err == nil {
		t.Fatal("accepted NTFS junction as project root")
	}
	if _, err := ResolveProject(context.Background(), Environment{Getwd: func() (string, error) { return junctionRoot, nil }}, junctionRoot); err == nil {
		t.Fatal("ResolveProject accepted NTFS junction project root")
	}

	// 2. Junction child ancestor inside a real workspace must be rejected before write.
	childJunction := filepath.Join(realDir, ".agents")
	createWindowsJunction(t, childJunction, otherDir)

	if _, err := ResolveTarget(realDir, ".agents/skills/x/SKILL.md"); err == nil {
		t.Fatal("accepted NTFS junction child ancestor")
	}

	// 3. Normal non-junction sibling inside the same workspace remains allowed.
	target, err := ResolveTarget(realDir, ".codex/hooks.json")
	if err != nil {
		t.Fatalf("normal sibling target failed unexpectedly: %v", err)
	}
	if filepath.Dir(target) != siblingDir {
		t.Fatalf("unexpected resolved sibling path: %s", target)
	}
}
