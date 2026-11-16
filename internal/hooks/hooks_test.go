package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func TestResolveStableExecutableAndQuoting(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stable := filepath.Join(dir, "my tools", "dtkit's.exe")
	if err := os.MkdirAll(filepath.Dir(stable), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stable, []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveStableExecutable(stable, nil, nil)
	if err != nil || got != stable {
		t.Fatalf("expected stable exe %q, got %q (%v)", stable, got, err)
	}
	if _, err := ResolveStableExecutable("relative/dtkit", nil, nil); apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected ExitUsage for relative exe, got %v", err)
	}
	ephemeral := filepath.Join(os.TempDir(), "go-build123", "exe", "dtkit")
	got, err = ResolveStableExecutable("", func() (string, error) { return ephemeral, nil }, func(string) (string, error) { return "", errors.New("none") })
	if apperr.ExitCodeOf(err) != apperr.ExitUsage || got != "" {
		t.Fatalf("expected ephemeral build path rejection, got %q err=%v", got, err)
	}

	projRoot := filepath.Join(dir, "dự án $proj & (1)")
	posix, win, err := FormatCommands(stable, "session-context", "project", projRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(posix, `'\''`) || !strings.HasPrefix(win, "& '") || !strings.Contains(win, "''") {
		t.Fatalf("unexpected quoting:\nposix=%s\nwin=%s", posix, win)
	}
}

func TestMergeAndUnmergePreservesUserHooks(t *testing.T) {
	existing := []byte(`{
  "customMeta": "keep",
  "hooks": {
    "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "echo pre"}]}],
    "SessionStart": [{"matcher": "startup", "hooks": [{"type": "command", "command": "echo user"}]}]
  }
}`)
	entry := BuildSessionContextEntry("'/bin/dtkit' hook run session-context --scope project --root '/repo'", "& 'C:\\bin\\dtkit.exe' hook run session-context --scope project --root 'C:\\repo'", 3)
	merged, dig, err := MergeHooksJSON(existing, true, entry)
	if err != nil {
		t.Fatal(err)
	}
	present, err := InspectHooksJSON(merged, dig)
	if err != nil || !present {
		t.Fatalf("expected hook present after merge: present=%v err=%v", present, err)
	}
	if !strings.Contains(string(merged), `"PreToolUse"`) || !strings.Contains(string(merged), `"echo user"`) || !strings.Contains(string(merged), `"customMeta"`) {
		t.Fatalf("user config lost during merge:\n%s", string(merged))
	}

	// Tampered entry triggers conflict on unmerge.
	tampered := bytes.Replace(merged, []byte("DTKit session context"), []byte("Edited"), 1)
	if _, _, err := UnmergeHooksJSON(tampered, dig, false); apperr.ExitCodeOf(err) != apperr.ExitConflict {
		t.Fatalf("expected conflict on modified DTKit hook entry, got %v", err)
	}

	// Clean unmerge keeps user hooks intact and never deletes pre-existing file.
	unmerged, deleteFile, err := UnmergeHooksJSON(merged, dig, false)
	if err != nil || deleteFile {
		t.Fatalf("unmerge failed: deleteFile=%v err=%v", deleteFile, err)
	}
	if !strings.Contains(string(unmerged), `"echo user"`) || strings.Contains(string(unmerged), "DTKit session context") {
		t.Fatalf("unexpected unmerged result:\n%s", string(unmerged))
	}

	// File created solely by DTKit is marked deletable when empty after unmerge.
	freshMerged, freshDig, err := MergeHooksJSON(nil, false, entry)
	if err != nil {
		t.Fatal(err)
	}
	_, delFresh, err := UnmergeHooksJSON(freshMerged, freshDig, true)
	if err != nil || !delFresh {
		t.Fatalf("expected deleteFile=true for DTKit-created empty hooks.json, got %v err=%v", delFresh, err)
	}
}

func TestRunSessionContextBoundsAndProtocol(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
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
	st := state.Installation{
		SchemaVersion:   2,
		ManagedBy:       "dtkit",
		Generation:      1,
		Scope:           "project",
		Runtime:         "codex",
		AdapterRevision: "1",
		InstalledAt:     "2026-11-16T00:00:00Z",
		UpdatedAt:       "2026-11-16T00:00:00Z",
		Kit:             state.Kit{ID: "engineer", Version: "0.1.0", Digest: strings.Repeat("a", 64)},
	}
	for _, a := range artifacts {
		rec := state.Artifact{Root: a.Root, Path: a.Path, Strategy: a.Strategy, Digest: strings.Repeat("a", 64)}
		if a.Strategy == "managed-block" {
			rec.Block = &state.BlockState{Digest: strings.Repeat("a", 64)}
		}
		st.Artifacts = append(st.Artifacts, rec)
	}
	if err := os.MkdirAll(filepath.Join(root, ".dtkit"), 0755); err != nil {
		t.Fatal(err)
	}
	stateBytes, _ := json.MarshalIndent(st, "", "  ")
	if err := os.WriteFile(filepath.Join(root, ".dtkit", "state.json"), stateBytes, 0600); err != nil {
		t.Fatal(err)
	}

	// Untrusted cwd in payload must be ignored; state is read strictly from root.
	payload := `{"hook_event_name":"SessionStart","source":"startup","session_id":"s1","cwd":"/tmp/evil-other-dir"}`
	var stdout, stderr bytes.Buffer
	if err := RunSessionContext(context.Background(), strings.NewReader(payload), &stdout, &stderr, "project", root); err != nil {
		t.Fatalf("RunSessionContext failed: %v", err)
	}
	var resp hookResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("invalid stdout JSON: %v\n%s", err, stdout.String())
	}
	if resp.HookSpecificOutput == nil || resp.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Fatalf("unexpected hook output: %+v", resp)
	}
	if !strings.Contains(resp.HookSpecificOutput.AdditionalContext, "dtkit-project-plan") || len(resp.HookSpecificOutput.AdditionalContext) > MaxAdditionalContextBytes {
		t.Fatalf("unexpected additionalContext: %q", resp.HookSpecificOutput.AdditionalContext)
	}

	// Reject oversized stdin (> 1 MiB).
	huge := strings.NewReader(strings.Repeat("x", MaxHookStdinBytes+10))
	stdout.Reset()
	if err := RunSessionContext(context.Background(), huge, &stdout, &stderr, "project", root); apperr.ExitCodeOf(err) != apperr.ExitUsage {
		t.Fatalf("expected ExitUsage for oversized stdin, got %v", err)
	}

	// Respect cancelled context.
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(2 * time.Millisecond)
	if err := RunSessionContext(ctx, strings.NewReader(payload), &stdout, &stderr, "project", root); apperr.ExitCodeOf(err) != apperr.ExitCancelled {
		t.Fatalf("expected ExitCancelled, got %v", err)
	}
}
