package installer

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

type RemoveOptions struct {
	Runtime string
	Hooks   *TxHooks
}

type RemoveResult struct {
	Removed       bool                `json:"removed"`
	TransactionID string              `json:"transactionId,omitempty"`
	Plan          planner.InstallPlan `json:"plan"`
}

// PlanRemove inspects Codex state and disk to build a safe removal plan without needing the catalog.
func PlanRemove(scope, kitID string, roots map[string]string) (planner.InstallPlan, *state.Installation, bool, error) {
	return PlanRemoveForRuntime(adapter.DefaultRuntime, scope, kitID, roots)
}

// PlanRemoveForRuntime inspects state and disk for runtime to build a safe removal plan.
func PlanRemoveForRuntime(runtimeName, scope, kitID string, roots map[string]string) (planner.InstallPlan, *state.Installation, bool, error) {
	plan, _, existing, exists, err := planRemoveWithRegistry(runtimeName, scope, kitID, roots)
	return plan, existing, exists, err
}

func planRemoveWithRegistry(runtimeName, scope, kitID string, roots map[string]string) (planner.InstallPlan, *state.Registry, *state.Installation, bool, error) {
	if scope != "project" && scope != "global" {
		return planner.InstallPlan{}, nil, nil, false, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	profile, err := runtimeProfile(runtimeName)
	if err != nil {
		return planner.InstallPlan{}, nil, nil, false, err
	}
	pending, lockStatus, err := InspectTransactionsForRuntime(profile.ID, scope, roots)
	if err != nil {
		return planner.InstallPlan{}, nil, nil, false, err
	}
	if len(pending) > 0 || lockStatus.Present {
		return planner.InstallPlan{}, nil, nil, false, apperr.WithHint(apperr.Conflict("recovery_required", "unfinished transaction or lock exists"), fmt.Sprintf("run dtkit recover --%s --runtime %s --dry-run", scope, profile.ID))
	}
	statePath, err := runtimeStateTarget(profile.ID, scope, roots)
	if err != nil {
		return planner.InstallPlan{}, nil, nil, false, err
	}
	reg, exists, err := state.LoadRegistryForRuntime(statePath, scope, profile.ID)
	if err != nil {
		return planner.InstallPlan{}, nil, nil, false, err
	}
	if !exists {
		return planner.InstallPlan{}, nil, nil, false, nil
	}
	if err := checkGlobalHomeBinding(profile, scope, reg.GlobalHome(), roots); err != nil {
		return planner.InstallPlan{}, reg, nil, true, err
	}
	existingVal, kitFound := reg.Kits[kitID]
	if !kitFound {
		if len(reg.Kits) == 1 {
			sole := reg.Kits[reg.KitIDs()[0]]
			return planner.InstallPlan{}, reg, &sole, true, apperr.Conflict("kit_id_mismatch", fmt.Sprintf("installed kit %q does not match requested %q", sole.Kit.ID, kitID))
		}
		return planner.InstallPlan{}, reg, nil, false, nil
	}
	existing := &existingVal

	plan, err := planner.PlanRemove(scope, roots, existing)
	if err != nil {
		return planner.InstallPlan{}, reg, existing, true, err
	}
	if len(reg.Kits) == 1 {
		for _, hookReg := range reg.AllHooks() {
			hAct, hErr := buildHookRemovalAction(roots, hookReg)
			if hErr != nil {
				plan.Conflicts++
				plan.Actions = append(plan.Actions, planner.Action{
					Root:   hookReg.Root,
					Path:   hookReg.Path,
					Kind:   "conflict",
					Reason: "hook_entry_conflict",
				})
			} else {
				plan.Actions = append(plan.Actions, planner.Action{
					Root:   hAct.Root,
					Path:   hAct.Path,
					Kind:   hAct.Kind,
					Reason: "unregister_hook",
				})
			}
		}
	}
	return plan, reg, existing, true, nil
}

// Remove executes a conflict-free removal transaction using state metadata only.
func Remove(scope, kitID string, roots map[string]string, now time.Time, opts RemoveOptions) (RemoveResult, error) {
	var res RemoveResult
	if now.IsZero() {
		now = time.Now()
	}
	plan, reg, existing, exists, err := planRemoveWithRegistry(opts.Runtime, scope, kitID, roots)
	res.Plan = plan
	if err != nil {
		return res, err
	}
	if !exists {
		return res, nil
	}
	if plan.Conflicts != 0 {
		return res, apperr.Conflict("remove_conflicts", fmt.Sprintf("remove found %d conflict(s)", plan.Conflicts))
	}

	byKey := map[string]state.Artifact{}
	for _, sa := range existing.Artifacts {
		byKey[sa.Root+"\x00"+sa.Path] = sa
	}
	ordered := append([]planner.Action(nil), plan.Actions...)
	sort.Slice(ordered, func(i, k int) bool {
		if ordered[i].Root != ordered[k].Root {
			return ordered[i].Root < ordered[k].Root
		}
		return ordered[i].Path < ordered[k].Path
	})

	var txActions []TxAction
	for _, act := range ordered {
		if act.Reason == "unregister_hook" || act.Reason == "hook_entry_conflict" {
			continue
		}
		sa, ok := byKey[act.Root+"\x00"+act.Path]
		if !ok {
			return res, fmt.Errorf("state artifact disappeared during remove")
		}
		target, err := paths.ResolveTarget(roots[act.Root], act.Path)
		if err != nil {
			return res, err
		}
		info, statErr := os.Lstat(target)
		if os.IsNotExist(statErr) {
			// Metadata-only forget for already-missing whole-file artifact.
			if sa.Strategy == "managed-block" {
				return res, apperr.WithPath(apperr.Conflict("managed_target_missing", "managed instruction file disappeared"), act.Path)
			}
			continue
		}
		if statErr != nil {
			return res, statErr
		}
		if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
			return res, apperr.WithPath(apperr.Conflict("target_not_regular", "target is not a regular file"), act.Path)
		}
		before, err := readRegularLimited(target, maxArtifactBytes)
		if err != nil {
			return res, err
		}
		if sa.Strategy == "whole-file" {
			if contentHash(before) != sa.Digest {
				return res, apperr.WithPath(apperr.Conflict("managed_file_modified", "managed file was modified"), act.Path)
			}
			txActions = append(txActions, TxAction{
				Root:     sa.Root,
				Path:     sa.Path,
				Kind:     "remove",
				Strategy: "whole-file",
				Existed:  true,
				Before:   before,
				Mode:     info.Mode().Perm(),
			})
			continue
		}

		// managed-block
		if sa.Block == nil {
			return res, apperr.WithPath(apperr.Conflict("invalid_block_state", "missing managed block metadata"), act.Path)
		}
		after, deleteFile, bErr := planner.RemoveBlockForRuntime(existing.Runtime, existing.Kit.ID, before, scope, *sa.Block)
		if bErr != nil {
			return res, apperr.WithPath(apperr.Conflict("managed_block_conflict", bErr.Error()), act.Path)
		}
		kind := "update"
		if deleteFile {
			kind = "remove"
			after = nil
		}
		txActions = append(txActions, TxAction{
			Root:     sa.Root,
			Path:     sa.Path,
			Kind:     kind,
			Strategy: "managed-block",
			Existed:  true,
			Before:   before,
			After:    after,
			Mode:     info.Mode().Perm(),
		})
	}

	removingLastKit := len(reg.Kits) == 1
	if removingLastKit {
		for _, hookReg := range reg.AllHooks() {
			hAct, hErr := buildHookRemovalAction(roots, hookReg)
			if hErr != nil {
				return res, hErr
			}
			txActions = append(txActions, hAct)
		}
	}

	txReq := TxRequest{
		Runtime:   existing.Runtime,
		Operation: "remove",
		Scope:     scope,
		KitID:     existing.Kit.ID,
		Roots:     roots,
		Actions:   txActions,
		Now:       now,
		Hooks:     opts.Hooks,
	}
	if removingLastKit {
		txReq.PrevState = existing
		txReq.NextState = nil
	} else {
		nextReg := cloneRegistry(*reg)
		delete(nextReg.Kits, existing.Kit.ID)
		nextReg.UpdatedAt = now.UTC().Format(time.RFC3339)
		txReq.PrevRegistry = reg
		txReq.NextRegistry = &nextReg
	}
	txRes, err := ExecuteTx(txReq)
	if err != nil {
		return res, err
	}
	res.Removed = true
	res.TransactionID = txRes.TransactionID
	return res, nil
}
