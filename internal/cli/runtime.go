package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/gemini"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/opencode"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func resolveRuntimeProfile(runtimeArg string) (adapter.RuntimeProfile, error) {
	p, err := adapter.MustProfile(runtimeArg)
	if err != nil {
		return adapter.RuntimeProfile{}, apperr.Usage("unsupported_runtime", err.Error())
	}
	return p, nil
}

func resolveRuntimeStatePath(profile adapter.RuntimeProfile, scope string, roots map[string]string) (string, error) {
	return paths.ResolveTarget(roots[scopeRoot(scope)], profile.StateRelPath)
}

func resolveScopeRoots(ctx context.Context, deps Deps, profile adapter.RuntimeProfile, scope, projectPath string) (map[string]string, []string, error) {
	roots := map[string]string{}
	if scope == "project" {
		res, err := paths.ResolveProject(ctx, deps.Env, projectPath)
		if err != nil {
			return nil, nil, err
		}
		roots["project"] = res.Root
		return roots, res.Warnings, nil
	}
	if scope != "global" {
		return nil, nil, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	switch profile.ID {
	case "codex":
		res, err := paths.ResolveGlobal(deps.Env)
		if err != nil {
			return nil, nil, err
		}
		roots["user-home"] = res.UserHome
		roots["codex-home"] = res.CodexHome
		return roots, res.Warnings, nil
	case "gemini":
		res, err := paths.ResolveGeminiGlobal(deps.Env)
		if err != nil {
			return nil, nil, err
		}
		roots["user-home"] = res.UserHome
		roots["gemini-home"] = res.GeminiHome
		return roots, res.Warnings, nil
	case "opencode":
		res, err := paths.ResolveOpenCodeGlobal(deps.Env)
		if err != nil {
			return nil, nil, err
		}
		roots["user-home"] = res.UserHome
		roots["opencode-home"] = res.OpenCodeHome
		return roots, res.Warnings, nil
	case "cursor":
		res, err := paths.ResolveUserHome(deps.Env)
		if err != nil {
			return nil, nil, err
		}
		roots["user-home"] = res.UserHome
		return roots, res.Warnings, nil
	case "claude":
		res, err := paths.ResolveClaudeGlobal(deps.Env)
		if err != nil {
			return nil, nil, err
		}
		roots["user-home"] = res.UserHome
		roots["claude-home"] = res.ClaudeHome
		return roots, res.Warnings, nil
	default:
		return nil, nil, apperr.Usage("unsupported_runtime", fmt.Sprintf("unsupported runtime %q", profile.ID))
	}
}

func describeRoots(roots map[string]string) string {
	keys := make([]string, 0, len(roots))
	for k := range roots {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, roots[k]))
	}
	return strings.Join(parts, ", ")
}

func checkRuntimeHomeBinding(profile adapter.RuntimeProfile, scope string, existing *state.Installation, roots map[string]string) error {
	if scope != "global" || profile.GlobalHomeKey == "" {
		return nil
	}
	recorded := existing.GlobalHome()
	current := roots[profile.GlobalHomeKey]
	if !sameAbsPath(recorded, current) {
		return apperr.Conflict("profile_mismatch", fmt.Sprintf("installed %s home %q does not match current %q", profile.DisplayName, recorded, current))
	}
	return nil
}

func doctorRuntimeSpecificChecks(profile adapter.RuntimeProfile, scope string, roots map[string]string) []DoctorCheck {
	var checks []DoctorCheck
	if instrLoc, ok := profile.Instructions[scope]; ok {
		instrRoot := roots[instrLoc.Root]
		if overrideRel := profile.OverrideCheckPath(scope, instrLoc.Path); overrideRel != "" {
			if overridePath, err := paths.ResolveTarget(instrRoot, overrideRel); err == nil {
				if info, statErr := os.Lstat(overridePath); statErr == nil && info.Size() > 0 {
					checks = append(checks, DoctorCheck{
						Name:    "instruction_override",
						Scope:   scope,
						Status:  "warn",
						Message: fmt.Sprintf("non-empty %s present at %s", overrideRel, overridePath),
						Hint:    fmt.Sprintf("%s takes precedence over %s in %s", overrideRel, instrLoc.Path, profile.DisplayName),
					})
				}
			}
		}
		if profile.ID == "gemini" {
			settingsPath := filepath.Join(roots[instrLoc.Root], ".gemini", "settings.json")
			if scope == "global" {
				settingsPath = filepath.Join(roots["gemini-home"], "settings.json")
			}
			_, warn, err := gemini.InspectSettingsContextOverride(settingsPath)
			if err != nil {
				checks = append(checks, DoctorCheck{
					Name:    "context_settings",
					Scope:   scope,
					Status:  "warn",
					Message: err.Error(),
				})
			} else if warn != "" {
				checks = append(checks, DoctorCheck{
					Name:    "context_settings",
					Scope:   scope,
					Status:  "warn",
					Message: warn,
					Hint:    "include \"GEMINI.md\" in context.fileName so Gemini CLI loads DTKit managed instructions",
				})
			}
		}
	}
	if profile.ID == "opencode" {
		var candidates []string
		if scope == "project" {
			candidates = []string{
				filepath.Join(roots["project"], "opencode.json"),
				filepath.Join(roots["project"], ".opencode", "opencode.json"),
			}
		} else {
			candidates = []string{
				filepath.Join(roots["opencode-home"], "opencode.json"),
			}
		}
		for _, warn := range opencode.InspectConfigWarnings(candidates) {
			checks = append(checks, DoctorCheck{
				Name:    "opencode_config",
				Scope:   scope,
				Status:  "warn",
				Message: warn,
			})
		}
	}
	if capSet, ok := profile.CapabilitiesMap[scope]; ok {
		if !capSet.Instructions.DTKitSupported && capSet.Instructions.Note != "" {
			checks = append(checks, DoctorCheck{
				Name:    "instructions_capability",
				Scope:   scope,
				Status:  "warn",
				Message: capSet.Instructions.Note,
				Hint:    fmt.Sprintf("configure global instructions manually in %s UI if needed", profile.DisplayName),
			})
		}
		if !capSet.Hooks.DTKitSupported && capSet.Hooks.Note != "" {
			checks = append(checks, DoctorCheck{
				Name:    "hooks_capability",
				Scope:   scope,
				Status:  "pass",
				Message: capSet.Hooks.Note,
			})
		}
	}
	return checks
}

func appendUniqueWarning(warnings *[]string, msg string) {
	if msg == "" {
		return
	}
	for _, existing := range *warnings {
		if existing == msg {
			return
		}
	}
	*warnings = append(*warnings, msg)
}

// applyScopeCapabilitiesAndBinaryWarnings populates consistent capability metadata, integration status,
// missing-binary warnings, and capability fallback warnings across preview, apply, noop, info, and status.
func applyScopeCapabilitiesAndBinaryWarnings(deps Deps, profile adapter.RuntimeProfile, scope string, pkg *catalog.Package, env *Envelope) error {
	capPlan, err := adapter.EvaluateCapabilities(profile, scope, pkg)
	if err != nil {
		return err
	}
	capCopy := capPlan.Capabilities
	integCopy := capPlan.Integration
	env.Capabilities = &capCopy
	env.Integration = &integCopy
	for _, w := range capPlan.Warnings {
		appendUniqueWarning(&env.Warnings, w)
	}
	if !capPlan.Capabilities.Instructions.DTKitSupported && capPlan.Capabilities.Instructions.Note != "" {
		appendUniqueWarning(&env.Warnings, capPlan.Capabilities.Instructions.Note)
	}
	if _, lookErr := deps.LookPath(profile.Binary); lookErr != nil {
		appendUniqueWarning(&env.Warnings, fmt.Sprintf("%s executable not found on PATH; files will still be installed", profile.Binary))
	}
	return nil
}

func runtimeIntegrationDoctorCheck(profile adapter.RuntimeProfile, scope string) DoctorCheck {
	integ := adapter.IntegrationEvidence{Status: adapter.IntegrationUnverified, Note: "runtime integration unverified"}
	if ev, ok := profile.IntegrationMap[scope]; ok {
		integ = ev
	}
	status := "warn"
	if integ.Status == adapter.IntegrationVerified {
		status = "pass"
	}
	return DoctorCheck{
		Name:    "runtime_integration",
		Scope:   scope,
		Status:  status,
		Message: fmt.Sprintf("%s (%s): %s", integ.Status, profile.ID, integ.Note),
	}
}
