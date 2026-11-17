package hooks

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/check"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
)

func TestClaudeSettingsMergeAndUnmergePreservesUserSettings(t *testing.T) {
	existing := []byte(`{
  "permissions": {"allow": ["Read", "Bash(go test:*)"]},
  "mcpServers": {"github": {"command": "npx"}},
  "env": {"FOO": "bar"},
  "hooks": {
    "PostToolUse": [{"matcher": "Write", "hooks": [{"type": "command", "command": "echo user-post"}]}]
  }
}`)
	entry := BuildClaudeHookEntry("'/usr/local/bin/dtkit' hook run check-changed --scope project --root '/repo' --runtime claude", "Edit|Write|MultiEdit", 15, "DTKit changed-files quality check")
	merged, dig, err := MergeEventHookJSON(existing, true, EventPostToolUse, entry)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, dupErr := MergeEventHookJSON(merged, true, EventPostToolUse, entry); apperr.ExitCodeOf(dupErr) != apperr.ExitConflict {
		t.Fatalf("expected duplicate hook rejection, got %v", dupErr)
	}
	if present, err := InspectEventHooksJSON(merged, EventPostToolUse, dig); err != nil || !present {
		t.Fatalf("expected Claude hook present: present=%v err=%v", present, err)
	}
	for _, keep := range []string{`"permissions"`, `"mcpServers"`, `"env"`, `"echo user-post"`} {
		if !strings.Contains(string(merged), keep) {
			t.Fatalf("user setting %s lost in merged settings.json:\n%s", keep, string(merged))
		}
	}

	unmerged, delFile, err := UnmergeEventHookJSON(merged, EventPostToolUse, dig, false)
	if err != nil || delFile {
		t.Fatalf("unmerge failed: delFile=%v err=%v", delFile, err)
	}
	if !strings.Contains(string(unmerged), `"echo user-post"`) || strings.Contains(string(unmerged), "check-changed") {
		t.Fatalf("unexpected unmerged settings.json:\n%s", string(unmerged))
	}
}

func TestRunCheckChangedGuardsAndGuardEdit(t *testing.T) {
	repo, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module example.com/hk\ngo 1.27.4\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "main_test.go"), []byte("package hk\nimport \"testing\"\nfunc TestOK(t *testing.T){}\n"), 0644)

	prof := project.Profile{
		SchemaVersion: 1,
		Name:          "hk",
		Roots:         []project.WorkspaceRoot{{Path: ".", Stack: "go"}},
		Checks:        []project.CheckSpec{{ID: "go-test", Kind: "test", Executable: "go", Argv: []string{"test", "./..."}, Cwd: ".", TimeoutSec: 60}},
	}
	rawProf, _ := json.MarshalIndent(prof, "", "  ")
	_ = os.MkdirAll(filepath.Join(repo, ".dtkit"), 0755)
	_ = os.WriteFile(filepath.Join(repo, ".dtkit", "project.json"), append(rawProf, '\n'), 0644)

	baseTime := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)

	// 1. Untrusted checks emit safe reminder instead of blindly executing.
	var out, errBuf bytes.Buffer
	if err := runCheckChangedWithNow(t.Context(), strings.NewReader(`{"hook_event_name":"PostToolUse","tool_name":"Edit"}`), &out, &errBuf, "project", repo, func() time.Time { return baseTime }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "dtkit check --yes") {
		t.Fatalf("expected untrusted check reminder, got %s", out.String())
	}

	// 2. Trust checks -> executes go test and outputs bounded PostToolUse context.
	if err := check.RecordTrust(repo, prof, baseTime); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runCheckChangedWithNow(t.Context(), strings.NewReader(`{"hook_event_name":"PostToolUse","tool_name":"Edit"}`), &out, &errBuf, "project", repo, func() time.Time { return baseTime.Add(5 * time.Second) }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "PostToolUse") || !strings.Contains(out.String(), "pass=1") {
		t.Fatalf("expected pass=1 PostToolUse output, got %s", out.String())
	}

	// 3. Debounce window (<2s) skips re-run cleanly.
	out.Reset()
	if err := runCheckChangedWithNow(t.Context(), strings.NewReader(`{"hook_event_name":"PostToolUse"}`), &out, &errBuf, "project", repo, func() time.Time { return baseTime.Add(5500 * time.Millisecond) }); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "{}" {
		t.Fatalf("expected debounced empty {}, got %s", out.String())
	}

	// 4. Recursion guard DTKIT_HOOK_ACTIVE=1 short-circuits immediately.
	t.Setenv("DTKIT_HOOK_ACTIVE", "1")
	out.Reset()
	if err := runCheckChangedWithNow(t.Context(), strings.NewReader(`{"hook_event_name":"PostToolUse"}`), &out, &errBuf, "project", repo, func() time.Time { return baseTime.Add(30 * time.Second) }); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "{}" {
		t.Fatalf("expected recursion guard {}, got %s", out.String())
	}

	// 5. guard-edit blocks .env, .dtkit/local, traversal, and allows src/app.go.
	for _, tc := range []struct {
		file string
		deny bool
	}{
		{".env", true},
		{".env.production", true},
		{".env.example", false},
		{".dtkit/local/check-trust.json", true},
		{"../outside.txt", true},
		{"src/app.go", false},
	} {
		var gOut, gErr bytes.Buffer
		payload := `{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":"` + tc.file + `"}}`
		if err := RunGuardEdit(t.Context(), strings.NewReader(payload), &gOut, &gErr, "project", repo); err != nil {
			t.Fatalf("guard-edit error for %s: %v", tc.file, err)
		}
		hasDeny := strings.Contains(gOut.String(), `"permissionDecision":"deny"`)
		if hasDeny != tc.deny {
			t.Fatalf("guard-edit(%s) deny=%v want %v out=%s", tc.file, hasDeny, tc.deny, gOut.String())
		}
	}
}
