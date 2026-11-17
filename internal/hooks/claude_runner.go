package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/check"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const (
	hookDebounceWindow = 2 * time.Second
	hookCheckTimeout   = 12 * time.Second
	debounceRelPath    = ".dtkit/local/hook-debounce.json"
)

type claudeToolPayload struct {
	HookEventName string `json:"hook_event_name"`
	ToolName      string `json:"tool_name"`
	ToolInput     struct {
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
	} `json:"tool_input"`
}

type preToolUseOutput struct {
	HookSpecificOutput *preToolHookSpecific `json:"hookSpecificOutput,omitempty"`
}

type preToolHookSpecific struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	AdditionalContext        string `json:"additionalContext,omitempty"`
}

type debounceRecord struct {
	LastRunUnixMs int64 `json:"lastRunUnixMs"`
}

// RunCheckChanged executes opt-in PostToolUse checks with recursion, debounce, trust, and output guards.
func RunCheckChanged(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, scope, stateRoot string) error {
	return runCheckChangedWithNow(ctx, stdin, stdout, stderr, scope, stateRoot, time.Now)
}

func runCheckChangedWithNow(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, scope, stateRoot string, nowFn func() time.Time) error {
	if scope != "project" && scope != "global" {
		return apperr.Usage("invalid_scope", fmt.Sprintf("invalid hook scope %q", scope))
	}
	if stateRoot == "" {
		return apperr.Usage("missing_root", "--root is required")
	}
	if os.Getenv("DTKIT_HOOK_ACTIVE") == "1" {
		return writeEmptyHookOutput(stdout)
	}

	ctx, cancel := context.WithTimeout(ctx, hookCheckTimeout)
	defer cancel()

	if stdin != nil {
		raw, err := readLimitedWithCtx(ctx, stdin, MaxHookStdinBytes+1)
		if err != nil {
			return err
		}
		if len(raw) > MaxHookStdinBytes {
			return apperr.Usage("hook_stdin_too_large", "hook stdin payload exceeds 1 MiB limit")
		}
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) > 0 {
			var p claudeToolPayload
			if err := json.Unmarshal(trimmed, &p); err != nil {
				return apperr.Wrap(apperr.ExitUsage, "invalid_hook_payload", fmt.Sprintf("invalid hook JSON input: %v", err), err)
			}
			if p.HookEventName != "" && p.HookEventName != EventPostToolUse {
				return apperr.Usage("unexpected_hook_event", fmt.Sprintf("expected %s event, got %q", EventPostToolUse, p.HookEventName))
			}
		}
	}

	if scope == "global" {
		// Global scope has no fixed repo root to check; return safely.
		return writeEmptyHookOutput(stdout)
	}

	if shouldDebounceHook(stateRoot, nowFn()) {
		return writeEmptyHookOutput(stdout)
	}

	env := paths.DefaultEnvironment()
	env.Getwd = func() (string, error) { return stateRoot, nil }
	rep, err := check.Run(ctx, env, check.Options{
		Path:        stateRoot,
		ChangedOnly: true,
		Yes:         false, // Never silently bypass untrusted repo checks in a hook.
		ExtraEnv:    []string{"DTKIT_HOOK_ACTIVE=1"},
		Now:         nowFn,
	})
	if err != nil {
		if appErr := apperr.AsError(err); appErr != nil {
			switch appErr.Code {
			case "missing_project_profile", "check_lock_held":
				return writeEmptyHookOutput(stdout)
			case "untrusted_project_checks":
				return encodeClaudePostToolContext(stdout, "DTKit check-changed skipped: review .dtkit/project.json via 'dtkit check --dry-run' and approve with 'dtkit check --yes'.")
			}
		}
		fmt.Fprintf(stderr, "dtkit hook check-changed: %v\n", err)
		return writeEmptyHookOutput(stdout)
	}
	recordHookDebounce(stateRoot, nowFn())

	var failedIDs []string
	for _, r := range rep.Results {
		if r.Status == check.StatusFail || r.Status == check.StatusMissingTool || r.Status == check.StatusTimeout {
			failedIDs = append(failedIDs, fmt.Sprintf("%s(%s)", r.ID, r.Status))
		}
	}
	if rep.Summary.Total == 0 || (rep.Summary.Pass == 0 && len(failedIDs) == 0) {
		return writeEmptyHookOutput(stdout)
	}
	msg := fmt.Sprintf("DTKit changed checks: pass=%d fail=%d missing_tool=%d timeout=%d skipped=%d",
		rep.Summary.Pass, rep.Summary.Fail, rep.Summary.MissingTool, rep.Summary.Timeout, rep.Summary.Skipped)
	if len(failedIDs) > 0 {
		msg += " issues=[" + strings.Join(failedIDs, ", ") + "]"
	}
	return encodeClaudePostToolContext(stdout, msg)
}

func encodeClaudePostToolContext(stdout io.Writer, msg string) error {
	if len(msg) > MaxAdditionalContextBytes {
		msg = msg[:MaxAdditionalContextBytes-3] + "..."
	}
	return json.NewEncoder(stdout).Encode(hookResponse{
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:     EventPostToolUse,
			AdditionalContext: msg,
		},
	})
}

// RunGuardEdit enforces a lightweight PreToolUse guardrail against editing secret files or escaping the repo root.
func RunGuardEdit(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, scope, stateRoot string) error {
	if scope != "project" && scope != "global" {
		return apperr.Usage("invalid_scope", fmt.Sprintf("invalid hook scope %q", scope))
	}
	if stateRoot == "" {
		return apperr.Usage("missing_root", "--root is required")
	}
	ctx, cancel := context.WithTimeout(ctx, DefaultHandlerTimeout)
	defer cancel()

	if stdin == nil {
		return writeEmptyHookOutput(stdout)
	}
	raw, err := readLimitedWithCtx(ctx, stdin, MaxHookStdinBytes+1)
	if err != nil {
		return err
	}
	if len(raw) > MaxHookStdinBytes {
		return apperr.Usage("hook_stdin_too_large", "hook stdin payload exceeds 1 MiB limit")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return writeEmptyHookOutput(stdout)
	}
	var p claudeToolPayload
	if err := json.Unmarshal(trimmed, &p); err != nil {
		return apperr.Wrap(apperr.ExitUsage, "invalid_hook_payload", fmt.Sprintf("invalid hook JSON input: %v", err), err)
	}
	if p.HookEventName != "" && p.HookEventName != EventPreToolUse {
		return apperr.Usage("unexpected_hook_event", fmt.Sprintf("expected %s event, got %q", EventPreToolUse, p.HookEventName))
	}

	targetFile := strings.TrimSpace(p.ToolInput.FilePath)
	if targetFile == "" {
		targetFile = strings.TrimSpace(p.ToolInput.Path)
	}
	if targetFile == "" || scope == "global" {
		return writeEmptyHookOutput(stdout)
	}

	reason := evaluateGuardEditTarget(stateRoot, targetFile)
	if reason == "" {
		return writeEmptyHookOutput(stdout)
	}
	return json.NewEncoder(stdout).Encode(preToolUseOutput{
		HookSpecificOutput: &preToolHookSpecific{
			HookEventName:            EventPreToolUse,
			PermissionDecision:       "deny",
			PermissionDecisionReason: reason,
		},
	})
}

func evaluateGuardEditTarget(stateRoot, targetFile string) string {
	cleanRoot := filepath.Clean(stateRoot)
	var rel string
	if filepath.IsAbs(targetFile) {
		r, err := filepath.Rel(cleanRoot, filepath.Clean(targetFile))
		if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			return fmt.Sprintf("blocked edit outside project root (%s)", targetFile)
		}
		rel = filepath.ToSlash(r)
	} else {
		rel = path.Clean(strings.ReplaceAll(targetFile, `\`, "/"))
		if rel == ".." || strings.HasPrefix(rel, "../") {
			return fmt.Sprintf("blocked path traversal target (%s)", targetFile)
		}
	}

	base := strings.ToLower(path.Base(rel))
	if base == ".env" || (strings.HasPrefix(base, ".env.") && !strings.HasSuffix(base, ".example") && !strings.HasSuffix(base, ".sample")) {
		return fmt.Sprintf("blocked edit to secret environment file %q", rel)
	}
	if rel == ".dtkit/local" || strings.HasPrefix(rel, ".dtkit/local/") || rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return fmt.Sprintf("blocked edit to protected internal directory %q", rel)
	}
	// ResolveTarget checks symlink/reparse containment if parent dirs exist cleanly.
	if _, err := paths.ResolveTarget(cleanRoot, rel); err != nil && !errors.Is(err, os.ErrNotExist) {
		if appErr := apperr.AsError(err); appErr != nil && (appErr.Code == "symlink_escape" || appErr.Code == "unsafe_path") {
			return fmt.Sprintf("blocked unsafe target path %q: %s", rel, appErr.Message)
		}
	}
	return ""
}

func shouldDebounceHook(repoRoot string, now time.Time) bool {
	p, err := paths.ResolveTarget(repoRoot, debounceRelPath)
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return false
	}
	var rec debounceRecord
	if json.Unmarshal(raw, &rec) != nil || rec.LastRunUnixMs <= 0 {
		return false
	}
	diff := now.UnixMilli() - rec.LastRunUnixMs
	return diff >= 0 && diff < hookDebounceWindow.Milliseconds()
}

func recordHookDebounce(repoRoot string, now time.Time) {
	p, err := paths.ResolveTarget(repoRoot, debounceRelPath)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0700)
	data, _ := json.Marshal(debounceRecord{LastRunUnixMs: now.UnixMilli()})
	_ = os.WriteFile(p, append(data, '\n'), 0600)
}
