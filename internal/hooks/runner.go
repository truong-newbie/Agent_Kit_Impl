package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

const (
	MaxHookStdinBytes         = 1024 * 1024 // 1 MiB
	MaxAdditionalContextBytes = 2048        // 2 KiB
	DefaultHandlerTimeout     = 3 * time.Second
)

type hookPayload struct {
	HookEventName string `json:"hook_event_name"`
	Source        string `json:"source"`
	SessionID     string `json:"session_id"`
	CWD           string `json:"cwd"`
}

type hookResponse struct {
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

// RunSessionContext implements the read-only SessionStart hook handler.
// It ignores payload cwd for filesystem access and enforces stdin/output/timeout bounds.
func RunSessionContext(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, scope, stateRoot string) error {
	if scope != "project" && scope != "global" {
		return apperr.Usage("invalid_scope", fmt.Sprintf("invalid hook scope %q", scope))
	}
	if stateRoot == "" {
		return apperr.Usage("missing_root", "--root is required")
	}

	ctx, cancel := context.WithTimeout(ctx, DefaultHandlerTimeout)
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
			var payload hookPayload
			if err := json.Unmarshal(trimmed, &payload); err != nil {
				return apperr.Wrap(apperr.ExitUsage, "invalid_hook_payload", fmt.Sprintf("invalid hook JSON input: %v", err), err)
			}
			if payload.HookEventName != "" && payload.HookEventName != EventSessionStart {
				return apperr.Usage("unexpected_hook_event", fmt.Sprintf("expected %s event, got %q", EventSessionStart, payload.HookEventName))
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return apperr.Cancelled("hook handler timed out or cancelled")
	}

	statePath, err := paths.ResolveTarget(stateRoot, ".dtkit/state.json")
	if err != nil {
		return err
	}
	inst, exists, err := state.LoadFile(statePath, scope)
	if err != nil {
		fmt.Fprintf(stderr, "dtkit hook session-context: %v\n", err)
		return writeEmptyHookOutput(stdout)
	}
	if !exists {
		return writeEmptyHookOutput(stdout)
	}

	msg := buildSessionContextMessage(inst)
	resp := hookResponse{
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:     EventSessionStart,
			AdditionalContext: msg,
		},
	}
	return json.NewEncoder(stdout).Encode(resp)
}

func writeEmptyHookOutput(w io.Writer) error {
	return json.NewEncoder(w).Encode(map[string]any{})
}

func buildSessionContextMessage(inst *state.Installation) string {
	var skillNames []string
	prefix := ".agents/skills/"
	for _, a := range inst.Artifacts {
		if strings.HasPrefix(a.Path, prefix) {
			skillNames = append(skillNames, path.Base(path.Dir(a.Path)))
		}
	}
	sort.Strings(skillNames)

	resourceDir := ".dtkit/content/" + inst.Kit.ID
	if inst.Scope == "global" {
		resourceDir = "$HOME/.dtkit/content/" + inst.Kit.ID
	}
	msg := fmt.Sprintf("DTKit %s %s (%s) active. Skills: %s. Consult %s for engineering rules, workflows, and templates.",
		inst.Kit.ID, inst.Kit.Version, inst.Scope, strings.Join(skillNames, ", "), resourceDir)
	if len(msg) > MaxAdditionalContextBytes {
		msg = msg[:MaxAdditionalContextBytes-3] + "..."
	}
	return msg
}

func readLimitedWithCtx(ctx context.Context, r io.Reader, limit int64) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(r, limit))
		ch <- result{data, err}
	}()
	select {
	case <-ctx.Done():
		return nil, apperr.Cancelled("hook stdin read cancelled or timed out")
	case res := <-ch:
		return res.data, res.err
	}
}
