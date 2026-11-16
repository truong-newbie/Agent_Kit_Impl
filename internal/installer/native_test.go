package installer

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
)

func TestUnicodeSpacesRootCRLFAndBOMPreservation(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj với dấu cách (dự án) #1")
	if err := os.MkdirAll(proj, 0755); err != nil {
		t.Fatal(err)
	}

	userHeader := "\xEF\xBB\xBF# User Header\r\nRule 1\r\n"
	agentsPath := filepath.Join(proj, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte(userHeader), 0644); err != nil {
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
		t.Fatalf("FreshInstall in Unicode/spaces root failed: %v", err)
	}

	afterInstall, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(afterInstall, []byte(userHeader)) {
		t.Fatalf("BOM or CRLF user header altered on install:\ngot:  %q\nwant prefix: %q", string(afterInstall), userHeader)
	}

	pkg2 := pkg
	pkg2.Version = "0.8.0"
	v2Artifacts := make([]adapter.Artifact, len(v1Artifacts))
	copy(v2Artifacts, v1Artifacts)
	for i := range v2Artifacts {
		if v2Artifacts[i].Strategy == "managed-block" {
			v2Artifacts[i].Content = bytes.Replace(v2Artifacts[i].Content, []byte("<!-- dtkit:end "), []byte("<!-- updated -->\n<!-- dtkit:end "), 1)
		}
	}
	res, err := Update("project", roots, pkg2, src, v2Artifacts, time.Now(), UpdateOptions{})
	if err != nil || !res.Updated {
		t.Fatalf("Update failed: res=%+v err=%v", res, err)
	}

	afterUpdate, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(afterUpdate, []byte(userHeader)) {
		t.Fatalf("BOM or CRLF user header altered on update:\ngot: %q", string(afterUpdate))
	}

	if _, err := Remove("project", "engineer", roots, time.Now(), RemoveOptions{}); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	afterRemove, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterRemove) != userHeader {
		t.Fatalf("original BOM+CRLF AGENTS.md not restored cleanly on remove:\ngot:  %q\nwant: %q", string(afterRemove), userHeader)
	}
}

func TestPOSIXReadOnlyParentConfinement(t *testing.T) {
	if runtime.GOOS == "windows" {
		return
	}
	if os.Geteuid() == 0 {
		t.Skip("POSIX DAC permission bits bypassed when running as root uid=0")
	}

	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
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

	restorePerms := func(paths ...string) {
		for _, p := range paths {
			_ = os.Chmod(p, 0755)
		}
	}

	t.Run("read-only operational parent .agents fails safely", func(t *testing.T) {
		proj := filepath.Join(base, "ro-agents")
		agentsDir := filepath.Join(proj, ".agents")
		if err := os.MkdirAll(agentsDir, 0755); err != nil {
			t.Fatal(err)
		}
		origAgentsMD := []byte("# Keep me\n")
		if err := os.WriteFile(filepath.Join(proj, "AGENTS.md"), origAgentsMD, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(agentsDir, 0555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { restorePerms(agentsDir) })

		if _, err := FreshInstall("project", map[string]string{"project": proj}, pkg, src, artifacts, time.Now()); err == nil {
			t.Fatal("expected failure on read-only .agents directory")
		}
		got, err := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
		if err != nil || string(got) != string(origAgentsMD) {
			t.Fatalf("AGENTS.md not rolled back/preserved on .agents 0555 failure: %q (%v)", string(got), err)
		}
	})

	t.Run("read-only state dir .dtkit fails safely", func(t *testing.T) {
		proj := filepath.Join(base, "ro-dtkit")
		dtkitDir := filepath.Join(proj, ".dtkit")
		localDir := filepath.Join(dtkitDir, "local")
		if err := os.MkdirAll(localDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(dtkitDir, 0555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { restorePerms(dtkitDir) })

		if _, err := FreshInstall("project", map[string]string{"project": proj}, pkg, src, artifacts, time.Now()); err == nil {
			t.Fatal("expected failure on read-only .dtkit directory")
		}
	})

	t.Run("read-only journal/lock parent .dtkit/local fails cleanly before mutating", func(t *testing.T) {
		proj := filepath.Join(base, "ro-local")
		localDir := filepath.Join(proj, ".dtkit", "local")
		if err := os.MkdirAll(localDir, 0755); err != nil {
			t.Fatal(err)
		}
		origAgentsMD := []byte("# Original\n")
		if err := os.WriteFile(filepath.Join(proj, "AGENTS.md"), origAgentsMD, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(localDir, 0555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { restorePerms(localDir) })

		if _, err := FreshInstall("project", map[string]string{"project": proj}, pkg, src, artifacts, time.Now()); err == nil {
			t.Fatal("expected failure on read-only .dtkit/local directory")
		}
		got, err := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
		if err != nil || string(got) != string(origAgentsMD) {
			t.Fatalf("AGENTS.md touched before lock/journal creation: %q (%v)", string(got), err)
		}
	})

	t.Run("read-only cache root rejects Cache.Put cleanly", func(t *testing.T) {
		cacheRoot := filepath.Join(base, "ro-cache")
		if err := os.MkdirAll(cacheRoot, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(cacheRoot, 0555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { restorePerms(cacheRoot) })

		c := dist.Cache{Root: cacheRoot}
		if _, err := c.Put([]byte("archive-bytes")); err == nil {
			t.Fatal("expected failure writing into 0555 cache root")
		}
	})
}

func TestLongPathBoundaryBehavior(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	deep := base
	for i := 0; i < 6; i++ {
		deep = filepath.Join(deep, strings.Repeat("d", 36))
	}
	if err := os.MkdirAll(deep, 0755); err != nil {
		// On Windows without long-path opt-in, OS MkdirAll itself may return path error safely.
		return
	}

	pkg, src, err := catalog.Engineer()
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := codex.Render(pkg, src, "project")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": deep}
	_, instErr := FreshInstall("project", roots, pkg, src, artifacts, time.Now())
	if instErr == nil {
		if st, err := Inspect("project", roots, artifacts); err != nil || !st.Healthy {
			t.Fatalf("long-path inspect failed after install success: %+v err=%v", st, err)
		}
		if _, err := Remove("project", "engineer", roots, time.Now(), RemoveOptions{}); err != nil {
			t.Fatalf("long-path remove failed: %v", err)
		}
	}
}

func TestMaliciousJournalTraversalAndCorruptStateBytePreservation(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(base, "proj")
	secretPath := filepath.Join(base, "secret.txt")
	secretContent := []byte("TOP_SECRET_DO_NOT_TOUCH\n")
	if err := os.WriteFile(secretPath, secretContent, 0644); err != nil {
		t.Fatal(err)
	}
	notePath := filepath.Join(proj, "notes", "keep.txt")
	noteContent := []byte("UNRELATED_REPO_FILE\n")
	if err := os.MkdirAll(filepath.Dir(notePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notePath, noteContent, 0644); err != nil {
		t.Fatal(err)
	}

	// 1. Malicious journal backup path traversal must fail before reading/writing external files.
	txDir := filepath.Join(proj, ".dtkit", "local", "transactions", "txevil001")
	if err := os.MkdirAll(txDir, 0700); err != nil {
		t.Fatal(err)
	}
	malJournal := Journal{
		SchemaVersion: 1,
		ID:            "txevil001",
		Operation:     "install",
		Scope:         "project",
		Runtime:       "codex",
		KitID:         "engineer",
		Phase:         phaseApplying,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Actions: []JournalAction{{
			Root:     "project",
			Path:     "../secret.txt",
			Kind:     "update",
			Strategy: "whole-file",
			Existed:  true,
			Backup:   "../../secret.txt",
			Mode:     0644,
			Status:   "applied",
		}},
	}
	rawJ, _ := json.MarshalIndent(malJournal, "", "  ")
	if err := os.WriteFile(filepath.Join(txDir, "journal.json"), rawJ, 0600); err != nil {
		t.Fatal(err)
	}

	roots := map[string]string{"project": proj}
	if _, err := Recover("project", roots, "", false, time.Now()); err == nil {
		t.Fatal("expected Recover to reject path-traversal journal action/backup")
	}

	gotSecret, _ := os.ReadFile(secretPath)
	if !bytes.Equal(gotSecret, secretContent) {
		t.Fatalf("outside secret.txt mutated on malicious journal: %q", string(gotSecret))
	}
	gotNote, _ := os.ReadFile(notePath)
	if !bytes.Equal(gotNote, noteContent) {
		t.Fatalf("unrelated keep.txt mutated on malicious journal: %q", string(gotNote))
	}

	// Remove malicious tx so corrupt state checks run cleanly.
	_ = os.RemoveAll(txDir)

	// 2. Corrupt state.json cases leave unrelated files and user AGENTS.md byte-for-byte untouched.
	agentsPath := filepath.Join(proj, "AGENTS.md")
	userAgents := []byte("# Existing user rules\nKeep byte-for-byte.\n")
	if err := os.WriteFile(agentsPath, userAgents, 0644); err != nil {
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

	corruptStates := []string{
		`{truncated`,
		`{"schemaVersion":99,"managedBy":"dtkit","generation":1,"scope":"project","runtime":"codex","adapterRevision":"1"}`,
		`{"schemaVersion":1,"managedBy":"dtkit","generation":1,"scope":"project","runtime":"codex","adapterRevision":"1","kit":{"id":"engineer","version":"0.2.0","digest":"bad"}}`,
	}
	statePath := filepath.Join(proj, ".dtkit", "state.json")
	for _, cs := range corruptStates {
		if err := os.WriteFile(statePath, []byte(cs), 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := Inspect("project", roots, artifacts); err == nil {
			t.Fatalf("Inspect accepted corrupt state: %s", cs)
		}
		if _, err := Update("project", roots, pkg, src, artifacts, time.Now(), UpdateOptions{}); err == nil {
			t.Fatalf("Update accepted corrupt state: %s", cs)
		}
		if _, err := Remove("project", "engineer", roots, time.Now(), RemoveOptions{}); err == nil {
			t.Fatalf("Remove accepted corrupt state: %s", cs)
		}
		gotAgents, _ := os.ReadFile(agentsPath)
		if !bytes.Equal(gotAgents, userAgents) {
			t.Fatalf("user AGENTS.md altered under corrupt state %s", cs)
		}
		gotNote2, _ := os.ReadFile(notePath)
		if !bytes.Equal(gotNote2, noteContent) {
			t.Fatalf("unrelated keep.txt altered under corrupt state %s", cs)
		}
	}
}
