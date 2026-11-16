package planner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dtkit.local/dtkit/internal/adapter/codex"
)

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

// InstallPreview supports fresh installations only. Existing state, symlinks,
// and unowned files are conflicts; this function is not an apply operation.
func InstallPreview(scope string, roots map[string]string, artifacts []codex.Artifact) (InstallPlan, error) {
	var plan InstallPlan
	if scope != "project" && scope != "global" {
		return plan, fmt.Errorf("invalid scope")
	}
	stateRoot := "project"
	if scope == "global" {
		stateRoot = "user-home"
	}
	statePath, err := ResolveTarget(roots[stateRoot], ".dtkit/state.json")
	if err != nil {
		return plan, err
	}
	if _, err := os.Lstat(statePath); err == nil {
		return plan, fmt.Errorf("installation state exists; status and update are not yet implemented")
	} else if !os.IsNotExist(err) {
		return plan, err
	}
	seen := map[string]bool{}
	for _, a := range artifacts {
		if scope == "project" && a.Root != "project" {
			return plan, fmt.Errorf("unexpected artifact root")
		}
		if scope == "global" && a.Root != "user-home" && a.Root != "codex-home" {
			return plan, fmt.Errorf("unexpected artifact root")
		}
		if a.Strategy == "managed-block" && (a.Path != "AGENTS.md" || (scope == "global" && a.Root != "codex-home")) {
			return plan, fmt.Errorf("invalid instruction artifact")
		}
		if a.Strategy == "whole-file" && !(strings.HasPrefix(a.Path, ".agents/skills/dtkit-"+scope+"-") || strings.HasPrefix(a.Path, ".dtkit/content/engineer/")) {
			return plan, fmt.Errorf("artifact outside allowed paths")
		}
		if scope == "global" && a.Strategy == "whole-file" && a.Root != "user-home" {
			return plan, fmt.Errorf("invalid global artifact root")
		}
		if a.Strategy != "managed-block" && a.Strategy != "whole-file" {
			return plan, fmt.Errorf("unknown artifact strategy")
		}
		target, err := ResolveTarget(roots[a.Root], a.Path)
		if err != nil {
			return plan, err
		}
		key := strings.ToLower(target)
		if seen[key] {
			return plan, fmt.Errorf("duplicate artifact target")
		}
		seen[key] = true
		action := Action{Root: a.Root, Path: a.Path}
		if a.Strategy == "managed-block" {
			override, err := ResolveTarget(roots[a.Root], "AGENTS.override.md")
			if err != nil {
				return plan, err
			}
			if info, err := os.Lstat(override); err == nil && info.Size() > 0 {
				action.Kind, action.Reason = "conflict", "instruction_override"
			} else if err != nil && !os.IsNotExist(err) {
				return plan, err
			}
		}
		if action.Kind != "conflict" {
			info, err := os.Lstat(target)
			switch {
			case os.IsNotExist(err):
				action.Kind, action.Reason = "create", "target_missing"
			case err != nil:
				return plan, err
			case !info.Mode().IsRegular():
				action.Kind, action.Reason = "conflict", "target_not_regular"
			case a.Strategy == "whole-file":
				action.Kind, action.Reason = "conflict", "unowned_file"
			case info.Size() > 8*1024*1024:
				action.Kind, action.Reason = "conflict", "instruction_too_large"
			default:
				content, err := os.ReadFile(target)
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

// safeTarget rejects links in existing ancestors and does not create directories.
// It cannot guarantee race-free writes by other processes; there are no writes here.
func ResolveTarget(root, relative string) (string, error) {
	if root == "" || !fs.ValidPath(relative) || relative == "." || strings.ContainsAny(relative, `\:`) {
		return "", fmt.Errorf("invalid root or relative path")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Join(abs, filepath.FromSlash(relative))
	inside, err := filepath.Rel(abs, target)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
		return "", fmt.Errorf("target escapes root")
	}
	for current := target; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("symbolic link in target path: %s", current)
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return target, nil
}
