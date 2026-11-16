package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/hooks"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

type HookStatusInfo struct {
	ID             string `json:"id"`
	Event          string `json:"event"`
	Scope          string `json:"scope"`
	Enabled        bool   `json:"enabled"`
	Status         string `json:"status"` // disabled | configured_pending_trust | broken_executable | missing | modified | conflicted
	Root           string `json:"root,omitempty"`
	Path           string `json:"path,omitempty"`
	Executable     string `json:"executable,omitempty"`
	Command        string `json:"command,omitempty"`
	CommandWindows string `json:"commandWindows,omitempty"`
	Detail         string `json:"detail,omitempty"`
	Hint           string `json:"hint,omitempty"`
}

func hookTarget(scope string) (rootKey, relPath string) {
	if scope == "global" {
		return "codex-home", "hooks.json"
	}
	return "project", ".codex/hooks.json"
}

// InspectHooks reports read-only status of opt-in DTKit hooks in scope.
func InspectHooks(scope string, roots map[string]string) ([]HookStatusInfo, error) {
	if scope != "project" && scope != "global" {
		return nil, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	statePath, err := paths.ResolveTarget(roots[scopeRoot(scope)], ".dtkit/state.json")
	if err != nil {
		return nil, err
	}
	existing, exists, err := state.LoadFile(statePath, scope)
	if err != nil {
		return nil, err
	}
	if !exists || len(existing.Hooks) == 0 {
		return []HookStatusInfo{{
			ID:     "session-context",
			Event:  hooks.EventSessionStart,
			Scope:  scope,
			Status: "disabled",
			Detail: "hook is opt-in and currently disabled",
		}}, nil
	}

	out := make([]HookStatusInfo, 0, len(existing.Hooks))
	for _, reg := range existing.Hooks {
		st := HookStatusInfo{
			ID:             reg.ID,
			Event:          reg.Event,
			Scope:          scope,
			Enabled:        true,
			Status:         "configured_pending_trust",
			Root:           reg.Root,
			Path:           reg.Path,
			Executable:     reg.Executable,
			Command:        reg.Command,
			CommandWindows: reg.CommandWindows,
			Hint:           "review and approve project/user hooks via /hooks in Codex CLI",
		}
		target, rErr := paths.ResolveTarget(roots[reg.Root], reg.Path)
		if rErr != nil {
			st.Status = "conflicted"
			st.Detail = rErr.Error()
			out = append(out, st)
			continue
		}
		info, statErr := os.Lstat(target)
		if os.IsNotExist(statErr) {
			st.Status = "missing"
			st.Detail = "hooks.json is missing"
			out = append(out, st)
			continue
		}
		if statErr != nil {
			return nil, statErr
		}
		if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
			st.Status = "conflicted"
			st.Detail = "hooks.json is not a regular file"
			out = append(out, st)
			continue
		}
		data, readErr := readRegularLimited(target, hooks.MaxHooksJSONBytes)
		if readErr != nil {
			st.Status = "conflicted"
			st.Detail = readErr.Error()
			out = append(out, st)
			continue
		}
		present, inspectErr := hooks.InspectHooksJSON(data, reg.EntryDigest)
		if inspectErr != nil {
			st.Status = "conflicted"
			st.Detail = inspectErr.Error()
			out = append(out, st)
			continue
		}
		if !present {
			st.Status = "modified"
			st.Detail = "DTKit hook entry in hooks.json was modified or removed"
			out = append(out, st)
			continue
		}
		if exeInfo, exeErr := os.Lstat(reg.Executable); exeErr != nil || paths.IsReparseOrSymlink(exeInfo) || !exeInfo.Mode().IsRegular() {
			st.Status = "broken_executable"
			st.Detail = fmt.Sprintf("recorded executable %q is missing or invalid", reg.Executable)
			out = append(out, st)
			continue
		}
		out = append(out, st)
	}
	return out, nil
}

// EnableHook registers a built-in opt-in hook in hooks.json and migrates state to schemaVersion 2.
func EnableHook(scope, hookID string, roots, otherScopeRoots map[string]string, exePath string, dryRun bool, now time.Time, txHooks *TxHooks) (HookStatusInfo, bool, error) {
	var res HookStatusInfo
	if hookID != "session-context" {
		return res, false, apperr.Usage("unsupported_hook", fmt.Sprintf("unsupported hook %q (supported: session-context)", hookID))
	}
	if scope != "project" && scope != "global" {
		return res, false, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	if now.IsZero() {
		now = time.Now()
	}

	pending, lockStatus, err := InspectTransactions(scope, roots)
	if err != nil {
		return res, false, err
	}
	if len(pending) > 0 || lockStatus.Present {
		return res, false, apperr.WithHint(apperr.Conflict("recovery_required", "unfinished transaction or lock exists"), fmt.Sprintf("run dtkit recover --%s --dry-run", scope))
	}

	stateRootKey := scopeRoot(scope)
	statePath, err := paths.ResolveTarget(roots[stateRootKey], ".dtkit/state.json")
	if err != nil {
		return res, false, err
	}
	existing, exists, err := state.LoadFile(statePath, scope)
	if err != nil {
		return res, false, err
	}
	if !exists {
		return res, false, apperr.WithHint(apperr.Conflict("not_installed", fmt.Sprintf("DTKit is not installed in %s scope", scope)), fmt.Sprintf("run dtkit install engineer --%s --yes first", scope))
	}
	if scope == "global" && !samePath(existing.CodexHome, roots["codex-home"]) {
		return res, false, apperr.Conflict("profile_mismatch", fmt.Sprintf("installed Codex home %q does not match current %q", existing.CodexHome, roots["codex-home"]))
	}

	// Prevent silent duplicate execution across project and global scopes.
	if len(otherScopeRoots) > 0 {
		otherScope := "global"
		if scope == "global" {
			otherScope = "project"
		}
		if otherStatePath, rErr := paths.ResolveTarget(otherScopeRoots[scopeRoot(otherScope)], ".dtkit/state.json"); rErr == nil {
			if otherInst, otherExists, oErr := state.LoadFile(otherStatePath, otherScope); oErr == nil && otherExists {
				for _, h := range otherInst.Hooks {
					if h.ID == hookID {
						return res, false, apperr.WithHint(
							apperr.Conflict("hook_scope_conflict", fmt.Sprintf("hook %q is already enabled in %s scope; enabling both scopes causes duplicate execution", hookID, otherScope)),
							fmt.Sprintf("run dtkit hooks disable %s --%s first", hookID, otherScope),
						)
					}
				}
			}
		}
	}

	for _, h := range existing.Hooks {
		if h.ID == hookID {
			if h.Executable == exePath {
				statuses, sErr := InspectHooks(scope, roots)
				if sErr == nil && len(statuses) > 0 {
					res = statuses[0]
					if res.Status == "configured_pending_trust" {
						return res, false, nil
					}
					return res, false, apperr.Conflict("hook_drift", fmt.Sprintf("hook %q is already enabled but status is %s", hookID, res.Status))
				}
			}
			return res, false, apperr.Conflict("hook_already_enabled", fmt.Sprintf("hook %q is already enabled in %s scope", hookID, scope))
		}
	}

	posixCmd, winCmd, err := hooks.FormatCommands(exePath, hookID, scope, filepath.Clean(roots[stateRootKey]))
	if err != nil {
		return res, false, err
	}
	entry := hooks.BuildSessionContextEntry(posixCmd, winCmd, hooks.DefaultTimeoutSec)

	rootKey, relPath := hookTarget(scope)
	target, err := paths.ResolveTarget(roots[rootKey], relPath)
	if err != nil {
		return res, false, err
	}

	var before []byte
	targetExisted := false
	mode := os.FileMode(0644)
	if info, statErr := os.Lstat(target); statErr == nil {
		if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
			return res, false, apperr.WithPath(apperr.Conflict("target_not_regular", "hooks.json is not a regular file"), relPath)
		}
		targetExisted = true
		mode = info.Mode().Perm()
		before, err = readRegularLimited(target, hooks.MaxHooksJSONBytes)
		if err != nil {
			return res, false, err
		}
	} else if !os.IsNotExist(statErr) {
		return res, false, statErr
	}

	merged, digest, err := hooks.MergeHooksJSON(before, targetExisted, entry)
	if err != nil {
		return res, false, err
	}

	reg := state.HookRegistration{
		ID:             hookID,
		Event:          hooks.EventSessionStart,
		HandlerID:      hookID,
		Root:           rootKey,
		Path:           relPath,
		Command:        posixCmd,
		CommandWindows: winCmd,
		Executable:     exePath,
		EntryDigest:    digest,
		TimeoutSec:     hooks.DefaultTimeoutSec,
		CreatedFile:    !targetExisted,
	}

	res = HookStatusInfo{
		ID:             reg.ID,
		Event:          reg.Event,
		Scope:          scope,
		Enabled:        true,
		Status:         "configured_pending_trust",
		Root:           reg.Root,
		Path:           reg.Path,
		Executable:     reg.Executable,
		Command:        reg.Command,
		CommandWindows: reg.CommandWindows,
		Hint:           "open /hooks in Codex CLI to review and trust the registered hook",
	}
	if dryRun {
		return res, true, nil
	}

	nextState := *existing
	nextState.SchemaVersion = 2
	nextState.Generation = existing.Generation + 1
	nextState.UpdatedAt = now.UTC().Format(time.RFC3339)
	nextState.Hooks = append(append([]state.HookRegistration(nil), existing.Hooks...), reg)
	if err := nextState.ValidateSelf(); err != nil {
		return res, false, err
	}

	kind := "create"
	if targetExisted {
		kind = "update"
	}
	_, err = ExecuteTx(TxRequest{
		Operation: "update",
		Scope:     scope,
		KitID:     existing.Kit.ID,
		Roots:     roots,
		Actions: []TxAction{{
			Root:     rootKey,
			Path:     relPath,
			Kind:     kind,
			Strategy: "hooks-json",
			Existed:  targetExisted,
			Before:   before,
			After:    merged,
			Mode:     mode,
		}},
		PrevState: existing,
		NextState: &nextState,
		Now:       now,
		Hooks:     txHooks,
	})
	if err != nil {
		return res, false, err
	}
	return res, true, nil
}

// DisableHook unregisters a DTKit hook from hooks.json and state.json inside a P1 transaction.
func DisableHook(scope, hookID string, roots map[string]string, dryRun bool, now time.Time, txHooks *TxHooks) (HookStatusInfo, bool, error) {
	res := HookStatusInfo{ID: hookID, Event: hooks.EventSessionStart, Scope: scope, Status: "disabled"}
	if hookID != "session-context" {
		return res, false, apperr.Usage("unsupported_hook", fmt.Sprintf("unsupported hook %q", hookID))
	}
	if scope != "project" && scope != "global" {
		return res, false, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	if now.IsZero() {
		now = time.Now()
	}

	pending, lockStatus, err := InspectTransactions(scope, roots)
	if err != nil {
		return res, false, err
	}
	if len(pending) > 0 || lockStatus.Present {
		return res, false, apperr.WithHint(apperr.Conflict("recovery_required", "unfinished transaction or lock exists"), fmt.Sprintf("run dtkit recover --%s --dry-run", scope))
	}

	statePath, err := paths.ResolveTarget(roots[scopeRoot(scope)], ".dtkit/state.json")
	if err != nil {
		return res, false, err
	}
	existing, exists, err := state.LoadFile(statePath, scope)
	if err != nil {
		return res, false, err
	}
	if !exists {
		return res, false, nil
	}
	if scope == "global" && !samePath(existing.CodexHome, roots["codex-home"]) {
		return res, false, apperr.Conflict("profile_mismatch", fmt.Sprintf("installed Codex home %q does not match current %q", existing.CodexHome, roots["codex-home"]))
	}

	idx := -1
	for i, h := range existing.Hooks {
		if h.ID == hookID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return res, false, nil
	}
	reg := existing.Hooks[idx]
	res.Root = reg.Root
	res.Path = reg.Path

	action, err := buildHookRemovalAction(roots, reg)
	if err != nil {
		return res, false, err
	}
	if dryRun {
		return res, true, nil
	}

	nextState := *existing
	nextState.Generation = existing.Generation + 1
	nextState.UpdatedAt = now.UTC().Format(time.RFC3339)
	nextState.Hooks = append([]state.HookRegistration(nil), existing.Hooks[:idx]...)
	nextState.Hooks = append(nextState.Hooks, existing.Hooks[idx+1:]...)
	if err := nextState.ValidateSelf(); err != nil {
		return res, false, err
	}

	_, err = ExecuteTx(TxRequest{
		Operation: "update",
		Scope:     scope,
		KitID:     existing.Kit.ID,
		Roots:     roots,
		Actions:   []TxAction{action},
		PrevState: existing,
		NextState: &nextState,
		Now:       now,
		Hooks:     txHooks,
	})
	if err != nil {
		return res, false, err
	}
	return res, true, nil
}

func buildHookRemovalAction(roots map[string]string, reg state.HookRegistration) (TxAction, error) {
	var act TxAction
	target, err := paths.ResolveTarget(roots[reg.Root], reg.Path)
	if err != nil {
		return act, err
	}
	info, statErr := os.Lstat(target)
	if os.IsNotExist(statErr) {
		return act, apperr.WithPath(apperr.Conflict("hook_file_missing", "hooks.json missing while hook is registered"), reg.Path)
	}
	if statErr != nil {
		return act, statErr
	}
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return act, apperr.WithPath(apperr.Conflict("target_not_regular", "hooks.json is not a regular file"), reg.Path)
	}
	before, err := readRegularLimited(target, hooks.MaxHooksJSONBytes)
	if err != nil {
		return act, err
	}
	after, deleteFile, err := hooks.UnmergeHooksJSON(before, reg.EntryDigest, reg.CreatedFile)
	if err != nil {
		return act, err
	}
	kind := "update"
	if deleteFile {
		kind = "remove"
		after = nil
	}
	return TxAction{
		Root:     reg.Root,
		Path:     reg.Path,
		Kind:     kind,
		Strategy: "hooks-json",
		Existed:  true,
		Before:   before,
		After:    after,
		Mode:     info.Mode().Perm(),
	}, nil
}
