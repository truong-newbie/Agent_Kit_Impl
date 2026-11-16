package planner

import (
	"os"
	"sort"
	"strings"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/apperr"
	"dtkit.local/dtkit/internal/paths"
	"dtkit.local/dtkit/internal/state"
)

// PlanDesired reconciles existing state, on-disk files, and desired artifacts
// according to the DTKit planner decision matrix. It never writes to disk.
func PlanDesired(scope, kitID string, roots map[string]string, existing *state.Installation, desired []codex.Artifact) (InstallPlan, error) {
	var plan InstallPlan
	if scope != "project" && scope != "global" {
		return plan, apperr.Usage("invalid_scope", "invalid scope")
	}

	desiredByKey := map[string]codex.Artifact{}
	seenTarget := map[string]bool{}
	for _, a := range desired {
		if !state.IsAllowedArtifactPath(scope, kitID, a.Root, a.Path, a.Strategy) {
			return plan, apperr.WithPath(apperr.Usage("artifact_outside_allowlist", "desired artifact outside allowlist"), a.Path)
		}
		target, err := ResolveTarget(roots[a.Root], a.Path)
		if err != nil {
			return plan, err
		}
		tKey := strings.ToLower(target)
		if seenTarget[tKey] {
			return plan, apperr.Usage("duplicate_artifact_target", "duplicate artifact target")
		}
		seenTarget[tKey] = true
		desiredByKey[strings.ToLower(a.Root+"\x00"+a.Path)] = a
	}

	stateByKey := map[string]state.Artifact{}
	if existing != nil {
		for _, sa := range existing.Artifacts {
			if !state.IsAllowedArtifactPath(scope, existing.Kit.ID, sa.Root, sa.Path, sa.Strategy) {
				return plan, apperr.WithPath(apperr.Conflict("invalid_state", "state artifact outside allowlist"), sa.Path)
			}
			stateByKey[strings.ToLower(sa.Root+"\x00"+sa.Path)] = sa
		}
	}

	allKeys := map[string]bool{}
	for k := range desiredByKey {
		allKeys[k] = true
	}
	for k := range stateByKey {
		allKeys[k] = true
	}

	for k := range allKeys {
		want, hasDesired := desiredByKey[k]
		recorded, hasState := stateByKey[k]

		rootKey := want.Root
		relPath := want.Path
		strategy := want.Strategy
		if !hasDesired {
			rootKey = recorded.Root
			relPath = recorded.Path
			strategy = recorded.Strategy
		}

		target, err := ResolveTarget(roots[rootKey], relPath)
		if err != nil {
			return plan, err
		}

		action := Action{Root: rootKey, Path: relPath}

		if strategy == "managed-block" && hasDesired {
			override, err := ResolveTarget(roots[rootKey], "AGENTS.override.md")
			if err != nil {
				return plan, err
			}
			if info, statErr := os.Lstat(override); statErr == nil && (paths.IsReparseOrSymlink(info) || info.Size() > 0) {
				action.Kind, action.Reason = "conflict", "instruction_override"
				plan.Conflicts++
				plan.Actions = append(plan.Actions, action)
				continue
			} else if statErr != nil && !os.IsNotExist(statErr) {
				return plan, statErr
			}
		}

		info, statErr := os.Lstat(target)
		diskExists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return plan, statErr
		}
		if diskExists {
			if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
				action.Kind, action.Reason = "conflict", "target_not_regular"
				plan.Conflicts++
				plan.Actions = append(plan.Actions, action)
				continue
			}
			if info.Size() > MaxInstructionBytes {
				action.Kind, action.Reason = "conflict", "target_too_large"
				plan.Conflicts++
				plan.Actions = append(plan.Actions, action)
				continue
			}
		}

		switch {
		case !hasState && hasDesired && !diskExists:
			action.Kind, action.Reason = "create", "target_missing"

		case !hasState && hasDesired && diskExists:
			if strategy == "whole-file" {
				action.Kind, action.Reason = "conflict", "unowned_file"
			} else {
				content, rErr := readLimited(target, MaxInstructionBytes)
				if rErr != nil {
					return plan, rErr
				}
				if _, _, bErr := AppendBlockForKit(kitID, content, true, want.Content, scope); bErr != nil {
					action.Kind, action.Reason = "conflict", "invalid_or_unowned_block"
				} else {
					action.Kind, action.Reason = "update", "append_managed_block"
				}
			}

		case hasState && hasDesired && !diskExists:
			action.Kind, action.Reason = "conflict", "managed_target_missing"

		case hasState && !hasDesired && !diskExists:
			action.Kind, action.Reason = "remove", "already_missing_metadata_only"

		case hasState && diskExists:
			content, rErr := readLimited(target, MaxInstructionBytes)
			if rErr != nil {
				return plan, rErr
			}
			if strategy == "whole-file" {
				if digest(content) != recorded.Digest {
					action.Kind, action.Reason = "conflict", "managed_file_modified"
				} else if !hasDesired {
					action.Kind, action.Reason = "remove", "removed_from_kit"
				} else if digest(want.Content) == recorded.Digest {
					action.Kind, action.Reason = "noop", "up_to_date"
				} else {
					action.Kind, action.Reason = "update", "content_changed"
				}
			} else {
				if recorded.Block == nil {
					action.Kind, action.Reason = "conflict", "invalid_block_state"
					break
				}
				actualBlockDigest, bErr := BlockDigestForKit(existing.Kit.ID, content, scope)
				if bErr != nil || actualBlockDigest != recorded.Digest {
					action.Kind, action.Reason = "conflict", "managed_block_modified"
				} else if !hasDesired {
					action.Kind, action.Reason = "remove", "removed_from_kit"
				} else {
					_, newState, repErr := ReplaceBlockForKit(kitID, content, want.Content, scope, *recorded.Block)
					if repErr != nil {
						action.Kind, action.Reason = "conflict", "invalid_or_unowned_block"
					} else if newState.Digest == recorded.Digest {
						action.Kind, action.Reason = "noop", "up_to_date"
					} else {
						action.Kind, action.Reason = "update", "content_changed"
					}
				}
			}
		}

		if action.Kind == "conflict" {
			plan.Conflicts++
		}
		plan.Actions = append(plan.Actions, action)
	}

	sort.Slice(plan.Actions, func(i, j int) bool {
		if plan.Actions[i].Root != plan.Actions[j].Root {
			return plan.Actions[i].Root < plan.Actions[j].Root
		}
		return plan.Actions[i].Path < plan.Actions[j].Path
	})
	return plan, nil
}

// PlanRemove plans uninstalling all artifacts recorded in existing state.
func PlanRemove(scope string, roots map[string]string, existing *state.Installation) (InstallPlan, error) {
	if existing == nil {
		return InstallPlan{}, nil
	}
	return PlanDesired(scope, existing.Kit.ID, roots, existing, nil)
}
