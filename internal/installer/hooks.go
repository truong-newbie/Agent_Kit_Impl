package installer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/hooks"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

type HookStatusInfo struct {
	ID             string `json:"id"`
	Runtime        string `json:"runtime,omitempty"`
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

func hookTargetForRuntime(profile adapter.RuntimeProfile, scope string) (rootKey, relPath string) {
	if scope == "global" {
		return profile.HookGlobalRoot, profile.HookGlobalPath
	}
	return "project", profile.HookProjectPath
}

func defaultHooksForRuntime(runtimeID, scope string) []HookStatusInfo {
	if runtimeID == "claude" {
		return []HookStatusInfo{
			{ID: "check-changed", Runtime: "claude", Event: hooks.EventPostToolUse, Scope: scope, Status: "disabled", Detail: "opt-in Claude PostToolUse changed-file check hook is disabled"},
			{ID: "guard-edit", Runtime: "claude", Event: hooks.EventPreToolUse, Scope: scope, Status: "disabled", Detail: "opt-in Claude PreToolUse edit guard hook is disabled"},
		}
	}
	return []HookStatusInfo{{
		ID:     "session-context",
		Event:  hooks.EventSessionStart,
		Scope:  scope,
		Status: "disabled",
		Detail: "hook is opt-in and currently disabled",
	}}
}

func hookReviewHint(runtimeID string) string {
	if runtimeID == "claude" {
		return "review registered hooks in .claude/settings.json (or $CLAUDE_CONFIG_DIR/settings.json) and run 'dtkit check --yes' before edits to trust repo checks"
	}
	return "review and approve project/user hooks via /hooks in Codex CLI"
}

// InspectHooks reports read-only status of opt-in DTKit Codex hooks in scope.
func InspectHooks(scope string, roots map[string]string) ([]HookStatusInfo, error) {
	return InspectHooksForRuntime(adapter.DefaultRuntime, scope, roots)
}

// InspectHooksForRuntime reports read-only status of opt-in DTKit hooks for runtimeName in scope.
func InspectHooksForRuntime(runtimeName, scope string, roots map[string]string) ([]HookStatusInfo, error) {
	if scope != "project" && scope != "global" {
		return nil, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	profile, err := runtimeProfile(runtimeName)
	if err != nil {
		return nil, err
	}
	if !profile.HooksSupported {
		return nil, apperr.Usage("hooks_not_supported", fmt.Sprintf("hooks are not supported for runtime %q", profile.ID))
	}
	statePath, err := runtimeStateTarget(profile.ID, scope, roots)
	if err != nil {
		return nil, err
	}
	existingReg, exists, err := state.LoadRegistryForRuntime(statePath, scope, profile.ID)
	if err != nil {
		return nil, err
	}
	hooksList := []state.HookRegistration(nil)
	if exists {
		hooksList = existingReg.AllHooks()
	}
	if !exists || len(hooksList) == 0 {
		return defaultHooksForRuntime(profile.ID, scope), nil
	}

	out := make([]HookStatusInfo, 0, len(hooksList))
	for _, reg := range hooksList {
		st := HookStatusInfo{
			ID:             reg.ID,
			Runtime:        profile.ID,
			Event:          reg.Event,
			Scope:          scope,
			Enabled:        true,
			Status:         "configured_pending_trust",
			Root:           reg.Root,
			Path:           reg.Path,
			Executable:     reg.Executable,
			Command:        reg.Command,
			CommandWindows: reg.CommandWindows,
			Hint:           hookReviewHint(profile.ID),
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
			st.Detail = reg.Path + " is missing"
			out = append(out, st)
			continue
		}
		if statErr != nil {
			return nil, statErr
		}
		if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
			st.Status = "conflicted"
			st.Detail = reg.Path + " is not a regular file"
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
		present, inspectErr := hooks.InspectEventHooksJSON(data, reg.Event, reg.EntryDigest)
		if inspectErr != nil {
			st.Status = "conflicted"
			st.Detail = inspectErr.Error()
			out = append(out, st)
			continue
		}
		if !present {
			st.Status = "modified"
			st.Detail = fmt.Sprintf("DTKit hook entry in %s was modified or removed", reg.Path)
			out = append(out, st)
			continue
		}
		if !hooks.IsValidExecutable(reg.Executable) {
			st.Status = "broken_executable"
			st.Detail = fmt.Sprintf("recorded executable %q is missing, invalid, or lacks execute permission", reg.Executable)
			out = append(out, st)
			continue
		}
		out = append(out, st)
	}
	return out, nil
}

// EnableHook registers a built-in opt-in Codex hook in hooks.json and migrates state to schemaVersion 2.
func EnableHook(scope, hookID string, roots, otherScopeRoots map[string]string, exePath string, dryRun bool, now time.Time, txHooks *TxHooks) (HookStatusInfo, bool, error) {
	return EnableHookForRuntime(adapter.DefaultRuntime, scope, hookID, roots, otherScopeRoots, exePath, dryRun, now, txHooks)
}

// EnableHookForRuntime registers a built-in opt-in hook for runtimeName and migrates state to schemaVersion 2.
func EnableHookForRuntime(runtimeName, scope, hookID string, roots, otherScopeRoots map[string]string, exePath string, dryRun bool, now time.Time, txHooks *TxHooks) (HookStatusInfo, bool, error) {
	var res HookStatusInfo
	profile, err := runtimeProfile(runtimeName)
	if err != nil {
		return res, false, err
	}
	if !profile.HooksSupported {
		return res, false, apperr.Usage("hooks_not_supported", fmt.Sprintf("hooks are not supported for runtime %q", profile.ID))
	}
	spec, err := hooks.ResolveHookSpec(profile.ID, hookID)
	if err != nil {
		return res, false, err
	}
	if scope != "project" && scope != "global" {
		return res, false, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	if now.IsZero() {
		now = time.Now()
	}

	pending, lockStatus, err := InspectTransactionsForRuntime(profile.ID, scope, roots)
	if err != nil {
		return res, false, err
	}
	if len(pending) > 0 || lockStatus.Present {
		return res, false, apperr.WithHint(apperr.Conflict("recovery_required", "unfinished transaction or lock exists"), fmt.Sprintf("run dtkit recover --%s --runtime %s --dry-run", scope, profile.ID))
	}

	stateRootKey := scopeRoot(scope)
	statePath, err := runtimeStateTarget(profile.ID, scope, roots)
	if err != nil {
		return res, false, err
	}
	existingReg, exists, err := state.LoadRegistryForRuntime(statePath, scope, profile.ID)
	if err != nil {
		return res, false, err
	}
	if !exists || len(existingReg.Kits) == 0 {
		return res, false, apperr.WithHint(apperr.Conflict("not_installed", fmt.Sprintf("DTKit is not installed in %s scope (%s)", scope, profile.ID)), fmt.Sprintf("run dtkit install engineer --%s --runtime %s --yes first", scope, profile.ID))
	}
	if err := checkGlobalHomeBinding(profile, scope, existingReg.GlobalHome(), roots); err != nil {
		return res, false, err
	}

	// Prevent silent duplicate execution across project and global scopes for the same runtime.
	if len(otherScopeRoots) > 0 {
		otherScope := "global"
		if scope == "global" {
			otherScope = "project"
		}
		if otherStatePath, rErr := runtimeStateTarget(profile.ID, otherScope, otherScopeRoots); rErr == nil {
			if otherReg, otherExists, oErr := state.LoadRegistryForRuntime(otherStatePath, otherScope, profile.ID); oErr == nil && otherExists {
				for _, h := range otherReg.AllHooks() {
					if h.ID == hookID {
						return res, false, apperr.WithHint(
							apperr.Conflict("hook_scope_conflict", fmt.Sprintf("hook %q is already enabled in %s scope (%s); enabling both scopes causes duplicate execution", hookID, otherScope, profile.ID)),
							fmt.Sprintf("run dtkit hooks disable %s --%s --runtime %s first", hookID, otherScope, profile.ID),
						)
					}
				}
			}
		}
	}

	for _, h := range existingReg.AllHooks() {
		if h.ID == hookID {
			if h.Executable == exePath {
				statuses, sErr := InspectHooksForRuntime(profile.ID, scope, roots)
				if sErr == nil {
					for _, st := range statuses {
						if st.ID == hookID {
							res = st
							if res.Status == "configured_pending_trust" {
								return res, false, nil
							}
							return res, false, apperr.WithHint(
								apperr.Conflict("hook_drift", fmt.Sprintf("hook %q is already enabled but status is %s", hookID, res.Status)),
								fmt.Sprintf("run 'dtkit hooks disable %s --%s --runtime %s --yes' then re-enable with a healthy executable", hookID, scope, profile.ID),
							)
						}
					}
				}
			}
			return res, false, apperr.WithHint(
				apperr.Conflict("hook_already_enabled", fmt.Sprintf("hook %q is already enabled in %s scope (%s, executable %s)", hookID, scope, profile.ID, h.Executable)),
				fmt.Sprintf("run 'dtkit hooks disable %s --%s --runtime %s --yes' then re-enable with the new --executable path", hookID, scope, profile.ID),
			)
		}
	}

	posixCmd, winCmd, err := hooks.FormatCommandsForRuntime(profile.ID, exePath, hookID, scope, filepath.Clean(roots[stateRootKey]))
	if err != nil {
		return res, false, err
	}
	var entry map[string]any
	if profile.ID == "claude" {
		entry = hooks.BuildClaudeHookEntry(posixCmd, spec.Matcher, spec.TimeoutSec, spec.StatusMessage)
	} else {
		entry = hooks.BuildSessionContextEntry(posixCmd, winCmd, spec.TimeoutSec)
	}

	rootKey, relPath := hookTargetForRuntime(profile, scope)
	target, err := paths.ResolveTarget(roots[rootKey], relPath)
	if err != nil {
		return res, false, err
	}

	var before []byte
	targetExisted := false
	mode := os.FileMode(0644)
	if info, statErr := os.Lstat(target); statErr == nil {
		if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
			return res, false, apperr.WithPath(apperr.Conflict("target_not_regular", relPath+" is not a regular file"), relPath)
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

	merged, digest, err := hooks.MergeEventHookJSON(before, targetExisted, spec.Event, entry)
	if err != nil {
		return res, false, err
	}

	reg := state.HookRegistration{
		ID:             hookID,
		Event:          spec.Event,
		HandlerID:      hookID,
		Root:           rootKey,
		Path:           relPath,
		Command:        posixCmd,
		CommandWindows: winCmd,
		Executable:     exePath,
		EntryDigest:    digest,
		TimeoutSec:     spec.TimeoutSec,
		CreatedFile:    !targetExisted,
	}

	res = HookStatusInfo{
		ID:             reg.ID,
		Runtime:        profile.ID,
		Event:          reg.Event,
		Scope:          scope,
		Enabled:        true,
		Status:         "configured_pending_trust",
		Root:           reg.Root,
		Path:           reg.Path,
		Executable:     reg.Executable,
		Command:        reg.Command,
		CommandWindows: reg.CommandWindows,
		Hint:           hookReviewHint(profile.ID),
	}
	if dryRun {
		return res, true, nil
	}

	primaryKitID := existingReg.KitIDs()[0]
	kind := "create"
	if targetExisted {
		kind = "update"
	}
	txReq := TxRequest{
		Runtime:   profile.ID,
		Operation: "update",
		Scope:     scope,
		KitID:     primaryKitID,
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
		Now:   now,
		Hooks: txHooks,
	}
	if len(existingReg.Kits) >= 2 {
		nextReg := cloneRegistry(*existingReg)
		nextReg.UpdatedAt = now.UTC().Format(time.RFC3339)
		nextReg.Hooks = append(existingReg.AllHooks(), reg)
		txReq.PrevRegistry = existingReg
		txReq.NextRegistry = &nextReg
	} else {
		sole := existingReg.Kits[primaryKitID]
		nextState := sole
		nextState.SchemaVersion = 2
		nextState.Generation = sole.Generation + 1
		nextState.UpdatedAt = now.UTC().Format(time.RFC3339)
		nextState.Hooks = append(existingReg.AllHooks(), reg)
		if err := nextState.ValidateSelf(); err != nil {
			return res, false, err
		}
		txReq.PrevState = &sole
		txReq.NextState = &nextState
	}
	_, err = ExecuteTx(txReq)
	if err != nil {
		return res, false, err
	}
	return res, true, nil
}

// DisableHook unregisters a DTKit Codex hook from hooks.json and state.json inside a P1 transaction.
func DisableHook(scope, hookID string, roots map[string]string, dryRun bool, now time.Time, txHooks *TxHooks) (HookStatusInfo, bool, error) {
	return DisableHookForRuntime(adapter.DefaultRuntime, scope, hookID, roots, dryRun, now, txHooks)
}

// DisableHookForRuntime unregisters a DTKit hook for runtimeName inside a P1 transaction.
func DisableHookForRuntime(runtimeName, scope, hookID string, roots map[string]string, dryRun bool, now time.Time, txHooks *TxHooks) (HookStatusInfo, bool, error) {
	var res HookStatusInfo
	profile, err := runtimeProfile(runtimeName)
	if err != nil {
		return res, false, err
	}
	if !profile.HooksSupported {
		return res, false, apperr.Usage("hooks_not_supported", fmt.Sprintf("hooks are not supported for runtime %q", profile.ID))
	}
	spec, err := hooks.ResolveHookSpec(profile.ID, hookID)
	if err != nil {
		return res, false, err
	}
	res = HookStatusInfo{ID: hookID, Runtime: profile.ID, Event: spec.Event, Scope: scope, Status: "disabled"}
	if scope != "project" && scope != "global" {
		return res, false, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	if now.IsZero() {
		now = time.Now()
	}

	pending, lockStatus, err := InspectTransactionsForRuntime(profile.ID, scope, roots)
	if err != nil {
		return res, false, err
	}
	if len(pending) > 0 || lockStatus.Present {
		return res, false, apperr.WithHint(apperr.Conflict("recovery_required", "unfinished transaction or lock exists"), fmt.Sprintf("run dtkit recover --%s --runtime %s --dry-run", scope, profile.ID))
	}

	statePath, err := runtimeStateTarget(profile.ID, scope, roots)
	if err != nil {
		return res, false, err
	}
	existingReg, exists, err := state.LoadRegistryForRuntime(statePath, scope, profile.ID)
	if err != nil {
		return res, false, err
	}
	if !exists || len(existingReg.Kits) == 0 {
		return res, false, nil
	}
	if err := checkGlobalHomeBinding(profile, scope, existingReg.GlobalHome(), roots); err != nil {
		return res, false, err
	}

	hooksList := existingReg.AllHooks()
	idx := -1
	for i, h := range hooksList {
		if h.ID == hookID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return res, false, nil
	}
	reg := hooksList[idx]
	res.Root = reg.Root
	res.Path = reg.Path

	action, err := buildHookRemovalAction(roots, reg)
	if err != nil {
		return res, false, err
	}
	if dryRun {
		return res, true, nil
	}

	primaryKitID := existingReg.KitIDs()[0]
	updatedHooks := append([]state.HookRegistration(nil), hooksList[:idx]...)
	updatedHooks = append(updatedHooks, hooksList[idx+1:]...)

	txReq := TxRequest{
		Runtime:   profile.ID,
		Operation: "update",
		Scope:     scope,
		KitID:     primaryKitID,
		Roots:     roots,
		Actions:   []TxAction{action},
		Now:       now,
		Hooks:     txHooks,
	}
	if len(existingReg.Kits) >= 2 {
		nextReg := cloneRegistry(*existingReg)
		nextReg.UpdatedAt = now.UTC().Format(time.RFC3339)
		nextReg.Hooks = updatedHooks
		txReq.PrevRegistry = existingReg
		txReq.NextRegistry = &nextReg
	} else {
		sole := existingReg.Kits[primaryKitID]
		nextState := sole
		nextState.Generation = sole.Generation + 1
		nextState.UpdatedAt = now.UTC().Format(time.RFC3339)
		nextState.Hooks = updatedHooks
		if err := nextState.ValidateSelf(); err != nil {
			return res, false, err
		}
		txReq.PrevState = &sole
		txReq.NextState = &nextState
	}
	_, err = ExecuteTx(txReq)
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
		return act, apperr.WithPath(apperr.Conflict("hook_file_missing", reg.Path+" missing while hook is registered"), reg.Path)
	}
	if statErr != nil {
		return act, statErr
	}
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return act, apperr.WithPath(apperr.Conflict("target_not_regular", reg.Path+" is not a regular file"), reg.Path)
	}
	before, err := readRegularLimited(target, hooks.MaxHooksJSONBytes)
	if err != nil {
		return act, err
	}
	after, deleteFile, err := hooks.UnmergeEventHookJSON(before, reg.Event, reg.EntryDigest, reg.CreatedFile)
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
