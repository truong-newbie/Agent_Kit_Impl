package installer

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

type RemoveOptions struct {
	Hooks *TxHooks
}

type RemoveResult struct {
	Removed       bool                `json:"removed"`
	TransactionID string              `json:"transactionId,omitempty"`
	Plan          planner.InstallPlan `json:"plan"`
}

// PlanRemove inspects state and disk to build a safe removal plan without needing the original catalog.
func PlanRemove(scope, kitID string, roots map[string]string) (planner.InstallPlan, *state.Installation, bool, error) {
	if scope != "project" && scope != "global" {
		return planner.InstallPlan{}, nil, false, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	pending, lockStatus, err := InspectTransactions(scope, roots)
	if err != nil {
		return planner.InstallPlan{}, nil, false, err
	}
	if len(pending) > 0 || lockStatus.Present {
		return planner.InstallPlan{}, nil, false, apperr.WithHint(apperr.Conflict("recovery_required", "unfinished transaction or lock exists"), fmt.Sprintf("run dtkit recover --%s --dry-run", scope))
	}
	statePath, err := paths.ResolveTarget(roots[scopeRoot(scope)], ".dtkit/state.json")
	if err != nil {
		return planner.InstallPlan{}, nil, false, err
	}
	existing, exists, err := state.LoadFile(statePath, scope)
	if err != nil {
		return planner.InstallPlan{}, nil, false, err
	}
	if !exists {
		return planner.InstallPlan{}, nil, false, nil
	}
	if existing.Kit.ID != kitID {
		return planner.InstallPlan{}, existing, true, apperr.Conflict("kit_id_mismatch", fmt.Sprintf("installed kit %q does not match requested %q", existing.Kit.ID, kitID))
	}
	if scope == "global" && !samePath(existing.CodexHome, roots["codex-home"]) {
		return planner.InstallPlan{}, existing, true, apperr.Conflict("profile_mismatch", fmt.Sprintf("installed Codex home %q does not match current %q", existing.CodexHome, roots["codex-home"]))
	}

	plan, err := planner.PlanRemove(scope, roots, existing)
	if err != nil {
		return planner.InstallPlan{}, existing, true, err
	}
	for _, reg := range existing.Hooks {
		hAct, hErr := buildHookRemovalAction(roots, reg)
		if hErr != nil {
			plan.Conflicts++
			plan.Actions = append(plan.Actions, planner.Action{
				Root:   reg.Root,
				Path:   reg.Path,
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
	return plan, existing, true, nil
}

// Remove executes a conflict-free removal transaction using state metadata only.
func Remove(scope, kitID string, roots map[string]string, now time.Time, opts RemoveOptions) (RemoveResult, error) {
	var res RemoveResult
	if now.IsZero() {
		now = time.Now()
	}
	plan, existing, exists, err := PlanRemove(scope, kitID, roots)
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
		after, deleteFile, bErr := planner.RemoveBlockForKit(existing.Kit.ID, before, scope, *sa.Block)
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

	for _, reg := range existing.Hooks {
		hAct, hErr := buildHookRemovalAction(roots, reg)
		if hErr != nil {
			return res, hErr
		}
		txActions = append(txActions, hAct)
	}

	txRes, err := ExecuteTx(TxRequest{
		Operation: "remove",
		Scope:     scope,
		KitID:     existing.Kit.ID,
		Roots:     roots,
		Actions:   txActions,
		PrevState: existing,
		NextState: nil,
		Now:       now,
		Hooks:     opts.Hooks,
	})
	if err != nil {
		return res, err
	}
	res.Removed = true
	res.TransactionID = txRes.TransactionID
	return res, nil
}
