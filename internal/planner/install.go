package planner

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const MaxInstructionBytes = 8 * 1024 * 1024

// Action describes a proposed change; planning never writes to disk.
type Action struct {
	Root   string `json:"root"`
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

type InstallPlan struct {
	Actions   []Action `json:"actions"`
	Conflicts int      `json:"conflicts"`
}

// InstallPreview supports fresh Codex installations only.
func InstallPreview(scope string, roots map[string]string, artifacts []adapter.Artifact) (InstallPlan, error) {
	return InstallPreviewForRuntime(adapter.DefaultRuntime, scope, roots, artifacts)
}

// InstallPreviewForRuntime supports fresh installations for the specified runtime.
// Existing state, symlinks, and unowned files are conflicts; this function never writes to disk.
func InstallPreviewForRuntime(runtime, scope string, roots map[string]string, artifacts []adapter.Artifact) (InstallPlan, error) {
	var plan InstallPlan
	profile, ok := adapter.Profile(runtime)
	if !ok {
		return plan, apperr.Usage("unsupported_runtime", fmt.Sprintf("unsupported runtime %q", runtime))
	}
	if !profile.SupportsScope(scope) {
		return plan, apperr.Usage("invalid_scope", "invalid scope")
	}
	statePath, err := ResolveTarget(roots[adapter.ScopeStateRoot(scope)], profile.StateRelPath)
	if err != nil {
		return plan, err
	}
	if _, err := os.Lstat(statePath); err == nil {
		return plan, apperr.WithPath(apperr.Conflict("state_exists", "installation state already exists; use status to inspect or update/remove when available"), statePath)
	} else if !os.IsNotExist(err) {
		return plan, err
	}
	seen := map[string]bool{}
	instrLoc := profile.Instructions[scope]
	skillLoc := profile.Skills[scope]
	contentRoot := profile.ContentRoot[scope]
	for _, a := range artifacts {
		if !profile.AllowedRootForScope(scope, a.Root) {
			return plan, apperr.Usage("unexpected_artifact_root", "unexpected artifact root")
		}
		if a.Strategy == "managed-block" && (a.Path != instrLoc.Path || a.Root != instrLoc.Root) {
			return plan, apperr.Usage("invalid_instruction_artifact", "invalid instruction artifact")
		}
		if a.Strategy == "whole-file" {
			isSkill := a.Root == skillLoc.Root && profile.IsSkillArtifact(scope, a.Path)
			isContent := a.Root == contentRoot && strings.HasPrefix(a.Path, profile.ContentRelBase+"/")
			if !isSkill && !isContent {
				return plan, apperr.Usage("artifact_outside_allowlist", "artifact outside allowed paths")
			}
		}
		if a.Strategy != "managed-block" && a.Strategy != "whole-file" {
			return plan, apperr.Usage("unknown_artifact_strategy", "unknown artifact strategy")
		}
		target, err := ResolveTarget(roots[a.Root], a.Path)
		if err != nil {
			return plan, err
		}
		key := strings.ToLower(target)
		if seen[key] {
			return plan, apperr.Usage("duplicate_artifact_target", "duplicate artifact target")
		}
		seen[key] = true
		action := Action{Root: a.Root, Path: a.Path}
		if a.Strategy == "managed-block" {
			if overrideRel := profile.OverrideCheckPath(scope, a.Path); overrideRel != "" {
				override, err := ResolveTarget(roots[a.Root], overrideRel)
				if err != nil {
					return plan, err
				}
				if info, err := os.Lstat(override); err == nil && (paths.IsReparseOrSymlink(info) || info.Size() > 0) {
					action.Kind, action.Reason = "conflict", "instruction_override"
				} else if err != nil && !os.IsNotExist(err) {
					return plan, err
				}
			}
		}
		if action.Kind != "conflict" {
			info, err := os.Lstat(target)
			switch {
			case os.IsNotExist(err):
				action.Kind, action.Reason = "create", "target_missing"
			case err != nil:
				return plan, err
			case paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular():
				action.Kind, action.Reason = "conflict", "target_not_regular"
			case a.Strategy == "whole-file":
				action.Kind, action.Reason = "conflict", "unowned_file"
			case info.Size() > MaxInstructionBytes:
				action.Kind, action.Reason = "conflict", "instruction_too_large"
			default:
				content, err := readLimited(target, MaxInstructionBytes)
				if err != nil {
					return plan, err
				}
				if _, _, err := AppendBlock(content, true, a.Content, scope); err != nil {
					action.Kind, action.Reason = "conflict", "invalid_or_unowned_block"
				} else {
					action.Kind, action.Reason = "update", "append_managed_block"
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

func readLimited(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("file exceeds %d bytes: %s", maxBytes, path)
	}
	return data, nil
}

// ResolveTarget delegates to paths.ResolveTarget.
func ResolveTarget(root, relative string) (string, error) {
	return paths.ResolveTarget(root, relative)
}
