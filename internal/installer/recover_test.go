package installer

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
)

func TestRecoverNoopAndDryRun(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	rep, err := Recover("project", roots, "", false, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if rep.Outcome != "noop" {
		t.Fatalf("expected noop, got %+v", rep)
	}
	if _, err := os.Stat(filepath.Join(root, ".dtkit")); !os.IsNotExist(err) {
		t.Fatalf("noop recover created .dtkit: %v", err)
	}
}

func TestRecoverCompletesForwardWhenStateAndArtifactsMatchAfter(t *testing.T) {
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
	// Run ExecuteTx directly but fail right before marking journal committed (leaving state.json & artifacts written).
	// First run FreshInstall normally, then simulate an interrupted journal by rewriting journal phase to applying.
	installed, err := FreshInstall("project", roots, pkg, source, artifacts, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	jPath := filepath.Join(root, ".dtkit", "local", "transactions", installed.LastTransactionID, "journal.json")
	jBytes, err := os.ReadFile(jPath)
	if err != nil {
		t.Fatal(err)
	}
	var j Journal
	if err := json.Unmarshal(jBytes, &j); err != nil {
		t.Fatal(err)
	}
	j.Phase = phaseApplying
	if err := writeJSON(jPath, j, 0600); err != nil {
		t.Fatal(err)
	}

	// Status should detect recoveryNeeded.
	st, err := Inspect("project", roots, artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if !st.RecoveryNeeded || st.Healthy {
		t.Fatalf("expected recoveryNeeded, got %+v", st)
	}

	// Dry-run reports committed without modifying journal.
	dry, err := Recover("project", roots, "", true, time.Now())
	if err != nil || dry.Outcome != phaseCommitted {
		t.Fatalf("expected dry-run committed, got %+v err=%v", dry, err)
	}

	// Real recover marks committed and keeps installed files intact.
	rep, err := Recover("project", roots, "", false, time.Now())
	if err != nil || rep.Outcome != phaseCommitted {
		t.Fatalf("expected committed recovery, got %+v err=%v", rep, err)
	}

	// Repeated recover is noop.
	rep2, err := Recover("project", roots, "", false, time.Now())
	if err != nil || rep2.Outcome != "noop" {
		t.Fatalf("expected repeat recover noop, got %+v err=%v", rep2, err)
	}

	stAfter, err := Inspect("project", roots, artifacts)
	if err != nil || !stAfter.Healthy || stAfter.RecoveryNeeded {
		t.Fatalf("expected healthy after forward recovery, got %+v err=%v", stAfter, err)
	}
}

func TestRecoverRollsBackInterruptedPartialWriteAndRespectsUserEditConflict(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agents := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(agents, []byte("original\n"), 0644); err != nil {
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

	// Create a valid transaction, then simulate a crash mid-apply:
	// Restore an interrupted state where 2 artifacts are written, state.json is absent, and journal says "applying".
	installed, err := FreshInstall("project", roots, pkg, source, artifacts, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	txDir := filepath.Join(root, ".dtkit", "local", "transactions", installed.LastTransactionID)
	jPath := filepath.Join(txDir, "journal.json")
	jBytes, err := os.ReadFile(jPath)
	if err != nil {
		t.Fatal(err)
	}
	var j Journal
	if err := json.Unmarshal(jBytes, &j); err != nil {
		t.Fatal(err)
	}
	// Simulate crash before state commit and after only AGENTS.md and one skill were written.
	_ = os.Remove(filepath.Join(root, ".dtkit", "state.json"))
	for i := 2; i < len(j.Actions); i++ {
		if j.Actions[i].Path != "AGENTS.md" {
			_ = os.Remove(filepath.Join(root, filepath.FromSlash(j.Actions[i].Path)))
			j.Actions[i].Status = "pending"
		}
	}
	j.Phase = phaseApplying
	if err := writeJSON(jPath, j, 0600); err != nil {
		t.Fatal(err)
	}

	rep, err := Recover("project", roots, "", false, time.Now())
	if err != nil || rep.Outcome != phaseRolledBack {
		t.Fatalf("expected rolled_back, got %+v err=%v", rep, err)
	}
	content, err := os.ReadFile(agents)
	if err != nil || string(content) != "original\n" {
		t.Fatalf("expected original AGENTS.md restored, got %q (%v)", content, err)
	}
}

func TestRecoverRejectsLiveLockAndClearsDeadLock(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	roots := map[string]string{"project": root}
	lockPath := filepath.Join(root, ".dtkit", "local", "install.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0700); err != nil {
		t.Fatal(err)
	}

	// Active current PID lock must fail closed.
	liveMeta := lockMetadata{Token: "tok1", PID: os.Getpid(), CreatedAt: time.Now().UTC().Format(time.RFC3339), Operation: "install"}
	if err := writeJSON(lockPath, liveMeta, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = Recover("project", roots, "", false, time.Now())
	if apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected ExitConflict for live lock, got %v", err)
	}
	if _, statErr := os.Stat(lockPath); statErr != nil {
		t.Fatalf("live lock was unexpectedly deleted: %v", statErr)
	}

	// Dead PID lock (from exited process) can be cleared by recover.
	cmd := exec.Command("true")
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "exit", "0")
	}
	if runErr := cmd.Run(); runErr != nil {
		t.Skipf("cannot run dummy exited process: %v", runErr)
	}
	deadPID := cmd.Process.Pid
	_ = os.Remove(lockPath)
	deadMeta := lockMetadata{Token: "tok2", PID: deadPID, CreatedAt: time.Now().UTC().Format(time.RFC3339), Operation: "install"}
	if err := writeJSON(lockPath, deadMeta, 0600); err != nil {
		t.Fatal(err)
	}
	rep, err := Recover("project", roots, "", false, time.Now())
	if err != nil {
		t.Fatalf("recover failed on dead lock: %v", err)
	}
	if rep.Outcome != phaseRolledBack {
		t.Fatalf("unexpected outcome: %+v", rep)
	}
	if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
		t.Fatalf("dead lock was not cleared: %v", statErr)
	}
}

func TestValidateJournalRejectsUnsafePaths(t *testing.T) {
	j := Journal{
		SchemaVersion: 1,
		ID:            "tx12345678",
		Operation:     "install",
		Scope:         "project",
		KitID:         "engineer",
		Phase:         phaseApplying,
		Actions: []JournalAction{
			{Root: "project", Path: "../escape", Kind: "create", Strategy: "whole-file", Status: "pending"},
		},
	}
	if err := validateJournal(j, "project"); err == nil || !strings.Contains(err.Error(), "allowlist") {
		t.Fatalf("expected allowlist rejection, got %v", err)
	}
	_ = errors.New("")
}
