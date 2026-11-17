package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func TestMain(m *testing.M) {
	if outPath := os.Getenv("DTKIT_HELPER_ECHO_ARGS"); outPath != "" {
		raw, err := json.Marshal(os.Args[1:])
		if err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(outPath, raw, 0644); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func hasAppErrCode(err error, code string) bool {
	var ae *apperr.Error
	return errors.As(err, &ae) && ae.Code == code
}

func TestHooksJSONStrictParsingAndPreservation(t *testing.T) {
	entry := BuildSessionContextEntry("'/usr/local/bin/dtkit' hook run session-context", "", DefaultTimeoutSec)

	t.Run("rejects duplicate keys root and nested", func(t *testing.T) {
		cases := []string{
			`{"hooks":{},"hooks":{}}`,
			`{"hooks":{"SessionStart":[]},"meta":{"k":1,"k":2}}`,
			`{"hooks":{"SessionStart":[],"SessionStart":[]}}`,
		}
		for _, tc := range cases {
			if _, _, err := MergeHooksJSON([]byte(tc), true, entry); apperr.ExitCodeOf(err) != apperr.ExitConflict || !hasAppErrCode(err, "invalid_hooks_json") {
				t.Fatalf("expected invalid_hooks_json conflict for duplicate keys %s, got %v", tc, err)
			}
			if _, _, err := UnmergeHooksJSON([]byte(tc), "sha256:abc", false); apperr.ExitCodeOf(err) != apperr.ExitConflict || !hasAppErrCode(err, "invalid_hooks_json") {
				t.Fatalf("expected unmerge rejection for duplicate keys %s, got %v", tc, err)
			}
			if _, err := InspectHooksJSON([]byte(tc), "sha256:abc"); apperr.ExitCodeOf(err) != apperr.ExitConflict || !hasAppErrCode(err, "invalid_hooks_json") {
				t.Fatalf("expected inspect rejection for duplicate keys %s, got %v", tc, err)
			}
		}
	})

	t.Run("rejects oversize hooks json", func(t *testing.T) {
		oversizeSpaces := bytes.Repeat([]byte(" "), MaxHooksJSONBytes+1)
		oversizeJSON := append([]byte(`{"hooks":{},"pad":"`), bytes.Repeat([]byte("a"), MaxHooksJSONBytes)...)
		oversizeJSON = append(oversizeJSON, []byte(`"}`)...)
		for _, tc := range [][]byte{oversizeSpaces, oversizeJSON} {
			if _, _, err := MergeHooksJSON(tc, true, entry); !hasAppErrCode(err, "hooks_json_too_large") {
				t.Fatalf("expected hooks_json_too_large in MergeHooksJSON, got %v", err)
			}
			if _, err := InspectHooksJSON(tc, "sha256:abc"); !hasAppErrCode(err, "hooks_json_too_large") {
				t.Fatalf("expected hooks_json_too_large in InspectHooksJSON, got %v", err)
			}
			if _, _, err := UnmergeHooksJSON(tc, "sha256:abc", false); !hasAppErrCode(err, "hooks_json_too_large") {
				t.Fatalf("expected hooks_json_too_large in UnmergeHooksJSON, got %v", err)
			}
		}
	})

	t.Run("rejects invalid schema before write", func(t *testing.T) {
		cases := []string{
			`[]`,
			`{"hooks":"str"}`,
			`{"hooks":{"SessionStart":123}}`,
			`{"hooks":{"SessionStart":["not-obj"]}}`,
		}
		for _, tc := range cases {
			if _, _, err := MergeHooksJSON([]byte(tc), true, entry); apperr.ExitCodeOf(err) != apperr.ExitConflict {
				t.Fatalf("expected conflict error for invalid schema %s, got %v", tc, err)
			}
		}
	})

	t.Run("preserves large numbers unknown events and custom fields across merge and unmerge", func(t *testing.T) {
		const bigInt = "9007199254740993"
		seed := `{
  "customMeta": {"bigInt": ` + bigInt + `, "tag": "keep-me"},
  "hooks": {
    "PreToolUse": [{"matcher": "*", "limit": ` + bigInt + `}],
    "SessionStart": [{"matcher": "startup", "hooks": [{"type": "command", "command": "echo user"}]}],
    "Stop": [{"matcher": "*"}]
  }
}`
		merged, digest, err := MergeHooksJSON([]byte(seed), true, entry)
		if err != nil {
			t.Fatalf("MergeHooksJSON failed: %v", err)
		}
		mergedStr := string(merged)
		for _, want := range []string{bigInt, `"PreToolUse"`, `"Stop"`, `"customMeta"`, `"echo user"`} {
			if !strings.Contains(mergedStr, want) {
				t.Fatalf("merged JSON lost %s:\n%s", want, mergedStr)
			}
		}

		present, err := InspectHooksJSON(merged, digest)
		if err != nil || !present {
			t.Fatalf("expected digest present after merge, got present=%v err=%v", present, err)
		}

		unmerged, deleteFile, err := UnmergeHooksJSON(merged, digest, true)
		if err != nil {
			t.Fatalf("UnmergeHooksJSON failed: %v", err)
		}
		if deleteFile {
			t.Fatal("expected deleteFile=false when unrelated hooks/fields remain")
		}
		unmergedStr := string(unmerged)
		for _, want := range []string{bigInt, `"PreToolUse"`, `"Stop"`, `"customMeta"`, `"echo user"`} {
			if !strings.Contains(unmergedStr, want) {
				t.Fatalf("unmerged JSON lost %s:\n%s", want, unmergedStr)
			}
		}
		if strings.Contains(unmergedStr, "DTKit session context") {
			t.Fatalf("DTKit hook remained after unmerge:\n%s", unmergedStr)
		}
	})
}

func TestHooksAmbiguousDigestConflict(t *testing.T) {
	entry := BuildSessionContextEntry("'/usr/local/bin/dtkit' --hook session-context", "", DefaultTimeoutSec)
	merged, digest, err := MergeHooksJSON(nil, false, entry)
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(merged))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	hooksMap := doc["hooks"].(map[string]any)
	arr := hooksMap[EventSessionStart].([]any)
	// Duplicate the DTKit entry alongside a user hook so two items share the exact digest.
	arr = append(arr, arr[0], map[string]any{
		"matcher": "startup",
		"hooks":   []any{map[string]any{"type": "command", "command": "echo user"}},
	})
	hooksMap[EventSessionStart] = arr
	dupBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := InspectHooksJSON(dupBytes, digest); apperr.ExitCodeOf(err) != apperr.ExitConflict || !hasAppErrCode(err, "hook_entry_ambiguous") {
		t.Fatalf("expected hook_entry_ambiguous conflict in InspectHooksJSON, got %v", err)
	}
	out, del, err := UnmergeHooksJSON(dupBytes, digest, false)
	if apperr.ExitCodeOf(err) != apperr.ExitConflict || !hasAppErrCode(err, "hook_entry_ambiguous") {
		t.Fatalf("expected hook_entry_ambiguous conflict in UnmergeHooksJSON, got %v", err)
	}
	if del || len(out) != 0 {
		t.Fatalf("UnmergeHooksJSON must not return mutated bytes or deleteFile on ambiguity, got len=%d del=%v", len(out), del)
	}
}

func TestExecutableValidationAndShellQuotingExecution(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("IsValidExecutable and ResolveStableExecutable guards", func(t *testing.T) {
		if IsValidExecutable(filepath.Join(base, "missing")) {
			t.Fatal("missing file should not be valid")
		}
		if IsValidExecutable(base) {
			t.Fatal("directory should not be valid executable")
		}
		ephemeral := filepath.Join(base, "go-build123", "dtkit")
		if err := os.MkdirAll(filepath.Dir(ephemeral), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ephemeral, []byte("bin"), 0755); err != nil {
			t.Fatal(err)
		}
		if IsValidExecutable(ephemeral) {
			t.Fatal("go-build ephemeral path should not be valid")
		}
		dotTest := filepath.Join(base, "hooks.test")
		if err := os.WriteFile(dotTest, []byte("bin"), 0755); err != nil {
			t.Fatal(err)
		}
		if IsValidExecutable(dotTest) {
			t.Fatal(".test binary should not be valid")
		}

		nonExec := filepath.Join(base, "noexec")
		if err := os.WriteFile(nonExec, []byte("bin"), 0644); err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" {
			if IsValidExecutable(nonExec) {
				t.Fatal("0644 file should not be valid on POSIX")
			}
			if _, err := ResolveStableExecutable(nonExec, nil, nil); err == nil {
				t.Fatal("ResolveStableExecutable should reject non-executable on POSIX")
			}
		}

		validExe := filepath.Join(base, "dtkit-real")
		if err := os.WriteFile(validExe, []byte("bin"), 0755); err != nil {
			t.Fatal(err)
		}
		if !IsValidExecutable(validExe) {
			t.Fatal("expected regular 0755 binary to be valid")
		}

		symExe := filepath.Join(base, "dtkit-link")
		if err := os.Symlink(validExe, symExe); err == nil {
			if IsValidExecutable(symExe) {
				t.Fatal("symlink executable should be rejected")
			}
			if _, err := ResolveStableExecutable(symExe, nil, nil); err == nil {
				t.Fatal("ResolveStableExecutable should reject symlink")
			}
		}
	})

	t.Run("special character quoting and real shell execution", func(t *testing.T) {
		selfExe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		selfBytes, err := os.ReadFile(selfExe)
		if err != nil {
			t.Fatal(err)
		}

		specialSegment := "sp ' $DOLLAR `echo PWN` dự án"
		if runtime.GOOS != "windows" {
			specialSegment = `sp ' " $DOLLAR $(echo PWN) ` + "`echo PWN`" + ` dự án`
		}
		binDir := filepath.Join(base, "bin "+specialSegment)
		rootDir := filepath.Join(base, "root "+specialSegment)
		for _, d := range []string{binDir, rootDir} {
			if err := os.MkdirAll(d, 0755); err != nil {
				t.Fatal(err)
			}
		}

		exeName := "dtkit-helper"
		if runtime.GOOS == "windows" {
			exeName += ".exe"
		}
		copiedExe := filepath.Join(binDir, exeName)
		if err := os.WriteFile(copiedExe, selfBytes, 0755); err != nil {
			t.Fatal(err)
		}
		if !IsValidExecutable(copiedExe) {
			t.Fatalf("copied helper executable rejected: %s", copiedExe)
		}

		posixCmd, winCmd, err := FormatCommands(copiedExe, "session-context", "project", rootDir)
		if err != nil {
			t.Fatalf("FormatCommands failed: %v", err)
		}

		outFile := filepath.Join(base, "argv.json")
		wantArgs := []string{"hook", "run", "session-context", "--scope", "project", "--root", rootDir}

		if runtime.GOOS != "windows" {
			cmd := exec.Command("sh", "-c", posixCmd)
			cmd.Env = append(os.Environ(),
				"DTKIT_HELPER_ECHO_ARGS="+outFile,
				"DOLLAR=INJECTED",
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("sh -c failed: %v\ncmd=%s\nout=%s", err, posixCmd, out)
			}
			raw, err := os.ReadFile(outFile)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(wantArgs) {
				t.Fatalf("posix argv mismatch:\ngot:  %#v\nwant: %#v", got, wantArgs)
			}
			for i := range wantArgs {
				if got[i] != wantArgs[i] {
					t.Fatalf("posix argv[%d] mismatch:\ngot:  %q\nwant: %q", i, got[i], wantArgs[i])
				}
			}
		} else {
			psPath, lookErr := exec.LookPath("powershell")
			if lookErr != nil {
				t.Skip("powershell not present on PATH")
			}
			cmd := exec.Command(psPath, "-NoProfile", "-NonInteractive", "-Command", winCmd)
			cmd.Env = append(os.Environ(),
				"DTKIT_HELPER_ECHO_ARGS="+outFile,
				"DOLLAR=INJECTED",
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("powershell -Command failed: %v\ncmd=%s\nout=%s", err, winCmd, out)
			}
			raw, err := os.ReadFile(outFile)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(wantArgs) {
				t.Fatalf("windows argv mismatch:\ngot:  %#v\nwant: %#v", got, wantArgs)
			}
			for i := range wantArgs {
				if got[i] != wantArgs[i] {
					t.Fatalf("windows argv[%d] mismatch:\ngot:  %q\nwant: %q", i, got[i], wantArgs[i])
				}
			}
		}
	})
}

type blockingTestReader struct {
	entered   chan struct{}
	exited    chan struct{}
	closeOnce sync.Once
	closed    chan struct{}
}

func newBlockingTestReader() *blockingTestReader {
	return &blockingTestReader{
		entered: make(chan struct{}),
		exited:  make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (r *blockingTestReader) Read(p []byte) (int, error) {
	closeOnce := func() {
		select {
		case <-r.entered:
		default:
			close(r.entered)
		}
	}
	closeOnce()
	<-r.closed
	select {
	case <-r.exited:
	default:
		close(r.exited)
	}
	return 0, io.EOF
}

func (r *blockingTestReader) Close() error {
	r.closeOnce.Do(func() {
		close(r.closed)
	})
	return nil
}

func waitGoroutinesSettle(t *testing.T, maxTarget int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= maxTarget {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > maxTarget {
		t.Fatalf("goroutine leak detected: baseline <= %d, current=%d", maxTarget, got)
	}
}

func TestRunnerTimeoutCancelLeakAndErrorBounds(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projRoot := filepath.Join(base, "proj")
	evilRoot := filepath.Join(base, "evil-other-root")
	for _, d := range []string{projRoot, evilRoot} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("hung reader aborts on timeout with zero leaked goroutines", func(t *testing.T) {
		baseline := runtime.NumGoroutine()
		br := newBlockingTestReader()
		var stdout, stderr bytes.Buffer
		start := time.Now()
		err := runSessionContextWithTimeout(context.Background(), br, &stdout, &stderr, "project", projRoot, 35*time.Millisecond)
		elapsed := time.Since(start)
		if apperr.ExitCodeOf(err) != apperr.ExitCancelled {
			t.Fatalf("expected ExitCancelled on hung stdin timeout, got %v", err)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("timeout took too long: %s", elapsed)
		}
		select {
		case <-br.exited:
		case <-time.After(2 * time.Second):
			t.Fatal("blocked stdin reader did not unblock/exit after timeout")
		}
		waitGoroutinesSettle(t, baseline)
	})

	t.Run("hung os.Pipe aborts on cancel with zero leaked goroutines", func(t *testing.T) {
		baseline := runtime.NumGoroutine()
		pr, pw, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = pw.Close(); _ = pr.Close() }()

		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(25*time.Millisecond, cancel)

		var stdout, stderr bytes.Buffer
		err = runSessionContextWithTimeout(ctx, pr, &stdout, &stderr, "project", projRoot, 5*time.Second)
		if apperr.ExitCodeOf(err) != apperr.ExitCancelled {
			t.Fatalf("expected ExitCancelled on context cancel, got %v", err)
		}
		waitGoroutinesSettle(t, baseline)
	})

	t.Run("malformed payload and wrong event fail cleanly with empty stdout", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		err := RunSessionContext(context.Background(), strings.NewReader("{bad"), &stdout, &stderr, "project", projRoot)
		if apperr.ExitCodeOf(err) != apperr.ExitUsage || !hasAppErrCode(err, "invalid_hook_payload") {
			t.Fatalf("expected invalid_hook_payload usage error, got %v", err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("expected empty stdout on malformed payload, got %q", stdout.String())
		}

		stdout.Reset()
		stderr.Reset()
		err = RunSessionContext(context.Background(), strings.NewReader(`{"hook_event_name":"Stop","cwd":"/tmp"}`), &stdout, &stderr, "project", projRoot)
		if !hasAppErrCode(err, "unexpected_hook_event") {
			t.Fatalf("expected unexpected_hook_event, got %v", err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("expected empty stdout on unexpected event, got %q", stdout.String())
		}
	})

	t.Run("missing or corrupt state emits empty json object and ignores payload cwd", func(t *testing.T) {
		// Populate evilRoot with a DTKit state to verify payload cwd is never trusted.
		evilStatePath := filepath.Join(evilRoot, ".dtkit", "state.json")
		if err := os.MkdirAll(filepath.Dir(evilStatePath), 0755); err != nil {
			t.Fatal(err)
		}
		codexProfile, _ := adapter.Profile("codex")
		nowStr := time.Now().UTC().Format(time.RFC3339)
		evilDoc := state.Installation{
			SchemaVersion:   2,
			ManagedBy:       "dtkit",
			Generation:      1,
			Scope:           "project",
			Runtime:         "codex",
			AdapterRevision: codexProfile.Revision,
			Kit: state.Kit{
				ID:      "engineer",
				Version: "9.9.9",
				Digest:  strings.Repeat("a", 64),
			},
			Artifacts: []state.Artifact{{
				Root:     "project",
				Path:     "AGENTS.md",
				Strategy: "managed-block",
				Digest:   strings.Repeat("a", 64),
				Block:    &state.BlockState{Digest: strings.Repeat("a", 64)},
			}},
			InstalledAt: nowStr,
			UpdatedAt:   nowStr,
		}
		evilRaw, err := json.MarshalIndent(evilDoc, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(evilStatePath, evilRaw, 0644); err != nil {
			t.Fatal(err)
		}

		payload, _ := json.Marshal(map[string]any{
			"hook_event_name": "SessionStart",
			"cwd":             evilRoot,
		})

		// 1. Missing state.json in projRoot -> empty {} and no leakage from evilRoot.
		var stdout, stderr bytes.Buffer
		err = RunSessionContext(context.Background(), bytes.NewReader(payload), &stdout, &stderr, "project", projRoot)
		if err != nil {
			t.Fatalf("missing state.json should degrade gracefully, got %v", err)
		}
		if strings.TrimSpace(stdout.String()) != "{}" {
			t.Fatalf("expected {} on missing state, got %q", stdout.String())
		}
		if strings.Contains(stdout.String(), "9.9.9") {
			t.Fatalf("untrusted payload cwd leaked into stdout: %s", stdout.String())
		}

		// 2. Corrupt state.json in projRoot -> empty {} + stderr diagnostic.
		if err := os.MkdirAll(filepath.Join(projRoot, ".dtkit"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projRoot, ".dtkit", "state.json"), []byte("{corrupt"), 0644); err != nil {
			t.Fatal(err)
		}
		stdout.Reset()
		stderr.Reset()
		err = RunSessionContext(context.Background(), bytes.NewReader(payload), &stdout, &stderr, "project", projRoot)
		if err != nil {
			t.Fatalf("corrupt state.json should degrade gracefully to {}, got %v", err)
		}
		if strings.TrimSpace(stdout.String()) != "{}" {
			t.Fatalf("expected {} on corrupt state, got %q", stdout.String())
		}
		if stderr.Len() == 0 {
			t.Fatal("expected diagnostic message on stderr for corrupt state")
		}
	})
}
