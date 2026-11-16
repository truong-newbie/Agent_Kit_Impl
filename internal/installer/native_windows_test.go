//go:build windows

package installer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func createNativeJunction(t *testing.T, linkPath, targetDir string) {
	t.Helper()
	cmd := exec.Command("cmd", "/c", "mklink", "/J", linkPath, targetDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mklink /J %s -> %s failed: %v\n%s", linkPath, targetDir, err, out)
	}
}

func openExclusiveWindowsLock(t *testing.T, path string) syscall.Handle {
	t.Helper()
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	// shareMode = 0 enforces OS-level exclusive sharing lock (ERROR_SHARING_VIOLATION on competing opens/renames).
	h, err := syscall.CreateFile(
		ptr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		t.Fatalf("exclusive CreateFile(%s) failed: %v", path, err)
	}
	return h
}

func TestWindowsNTFSJunctionRejectsInstallerOperations(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj")
	outside := filepath.Join(base, "outside")
	for _, d := range []string{proj, outside} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, src, "project")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": proj}

	// Junction at .agents before install must fail cleanly without writing into outside dir.
	agentsJunction := filepath.Join(proj, ".agents")
	createNativeJunction(t, agentsJunction, outside)
	if _, err := FreshInstall("project", roots, pkg, src, artifacts, time.Now()); err == nil {
		t.Fatal("FreshInstall succeeded across .agents NTFS junction")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("outside junction target mutated: entries=%v err=%v", entries, err)
	}
	if err := os.Remove(agentsJunction); err != nil {
		t.Fatal(err)
	}

	// Install cleanly, then swap .dtkit/local to a junction and verify Update & Inspect fail closed.
	if _, err := FreshInstall("project", roots, pkg, src, artifacts, time.Now()); err != nil {
		t.Fatal(err)
	}
	localDir := filepath.Join(proj, ".dtkit", "local")
	if err := os.RemoveAll(localDir); err != nil {
		t.Fatal(err)
	}
	createNativeJunction(t, localDir, outside)
	if _, err := Update("project", roots, pkg, src, artifacts, time.Now(), UpdateOptions{}); err == nil {
		t.Fatal("Update succeeded across .dtkit/local NTFS junction")
	}
	if err := os.Remove(localDir); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsExclusiveFileLockRollsBackAndPreservesBytes(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj")
	if err := os.MkdirAll(proj, 0755); err != nil {
		t.Fatal(err)
	}

	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	v1Artifacts, err := codex.Render(pkg, src, "project")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": proj}
	if _, err := FreshInstall("project", roots, pkg, src, v1Artifacts, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Pick one regular file artifact (not AGENTS.md) and mutate both AGENTS.md and that file in v2.
	var lockedRel string
	v2Artifacts := make([]adapter.Artifact, len(v1Artifacts))
	copy(v2Artifacts, v1Artifacts)
	for i := range v2Artifacts {
		if v2Artifacts[i].Strategy == "managed-block" {
			v2Artifacts[i].Content = []byte(strings.Replace(string(v2Artifacts[i].Content), "<!-- dtkit:end ", "<!-- v2 -->\n<!-- dtkit:end ", 1))
		} else if lockedRel == "" && strings.HasSuffix(v2Artifacts[i].Path, ".md") {
			lockedRel = v2Artifacts[i].Path
			v2Artifacts[i].Content = append(append([]byte{}, v2Artifacts[i].Content...), []byte("\n# v2 skill update\n")...)
		}
	}
	if lockedRel == "" {
		t.Fatal("failed to locate skill artifact to lock")
	}

	lockedAbs := filepath.Join(proj, filepath.FromSlash(lockedRel))
	origBytes, err := os.ReadFile(lockedAbs)
	if err != nil {
		t.Fatal(err)
	}
	agentsPath := filepath.Join(proj, "AGENTS.md")
	origAgents, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}

	handle := openExclusiveWindowsLock(t, lockedAbs)
	defer func() {
		if handle != 0 {
			_ = syscall.CloseHandle(handle)
		}
	}()

	pkg2 := pkg
	pkg2.Version = "0.9.0"
	_, updateErr := Update("project", roots, pkg2, src, v2Artifacts, time.Now(), UpdateOptions{})
	if updateErr == nil {
		t.Fatal("expected Update to fail while target artifact is exclusively locked")
	}

	_ = syscall.CloseHandle(handle)
	handle = 0

	// Original locked file and sibling AGENTS.md must match pre-update bytes via rollback.
	afterLocked, err := os.ReadFile(lockedAbs)
	if err != nil || string(afterLocked) != string(origBytes) {
		t.Fatalf("locked artifact corrupted after failed update/rollback: err=%v len=%d vs %d", err, len(afterLocked), len(origBytes))
	}
	afterAgents, err := os.ReadFile(agentsPath)
	if err != nil || string(afterAgents) != string(origAgents) {
		t.Fatalf("AGENTS.md not rolled back after locked-file failure: %v", err)
	}

	// Clean retry after lock release must succeed cleanly.
	res, err := Update("project", roots, pkg2, src, v2Artifacts, time.Now(), UpdateOptions{})
	if err != nil || !res.Updated {
		t.Fatalf("retry update after lock release failed: res=%+v err=%v", res, err)
	}
}

func TestWindowsCaseCollisionPreservesExistingContent(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj")
	if err := os.MkdirAll(proj, 0755); err != nil {
		t.Fatal(err)
	}

	// Pre-seed lowercase agents.md before DTKit manages AGENTS.md.
	lowerPath := filepath.Join(proj, "agents.md")
	userContent := "# Existing Windows lowercase agents notes\r\nDo not lose this line.\r\n"
	if err := os.WriteFile(lowerPath, []byte(userContent), 0644); err != nil {
		t.Fatal(err)
	}

	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, src, "project")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": proj}
	if _, err := FreshInstall("project", roots, pkg, src, artifacts, time.Now()); err != nil {
		t.Fatalf("FreshInstall with existing lowercase agents.md failed: %v", err)
	}

	mergedRaw, err := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mergedRaw), "Existing Windows lowercase agents notes") ||
		!strings.Contains(string(mergedRaw), "Do not lose this line.") {
		t.Fatalf("lowercase agents.md content lost during AGENTS.md install:\n%s", string(mergedRaw))
	}

	// Directory case conflict (.Agents existing as a regular file vs .agents directory) must fail cleanly instead of panicking or mutating.
	projConflict := filepath.Join(base, "proj-dir-case")
	if err := os.MkdirAll(projConflict, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projConflict, ".Agents"), []byte("user file\n"), 0644); err != nil {
		t.Fatal(err)
	}
	_, dirErr := FreshInstall("project", map[string]string{"project": projConflict}, pkg, src, artifacts, time.Now())
	if dirErr == nil || apperr.ExitCodeOf(dirErr) == 0 {
		t.Fatalf("expected clean error when .Agents file collides with .agents dir, got %v", dirErr)
	}
	kept, err := os.ReadFile(filepath.Join(projConflict, ".Agents"))
	if err != nil || string(kept) != "user file\n" {
		t.Fatalf("existing .Agents file altered on collision: %q err=%v", string(kept), err)
	}
}
