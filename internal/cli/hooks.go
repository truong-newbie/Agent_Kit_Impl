package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/hooks"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
)

func runHooks(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("hooks")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 {
		return fail(apperr.Usage("missing_subcommand", "usage: dtkit hooks <list|status|enable|disable> [options]"))
	}
	sub := args[0]
	env.Command = "hooks " + sub

	switch sub {
	case "list", "status":
		return runHooksInspect(ctx, sub, args[1:], deps, env, jsonOutput, fail)
	case "enable":
		return runHooksEnable(ctx, args[1:], deps, env, jsonOutput, fail)
	case "disable":
		return runHooksDisable(ctx, args[1:], deps, env, jsonOutput, fail)
	default:
		return fail(apperr.Usage("unknown_subcommand", fmt.Sprintf("unknown hooks subcommand %q", sub)))
	}
}

func runHooksInspect(ctx context.Context, sub string, args []string, deps Deps, env Envelope, jsonOutput bool, fail func(error) error) error {
	var scope, projectPath, runtimeArg string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose at most one of --project or --global"))
			}
			scope = "project"
		case "--global":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose at most one of --project or --global"))
			}
			scope = "global"
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--runtime":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--runtime requires a value"))
			}
			runtimeArg = args[i]
		case "--path":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a value"))
			}
			projectPath = args[i]
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown hooks %s option %q", sub, args[i])))
		}
	}
	profile, err := checkHooksRuntime(runtimeArg)
	if err != nil {
		return fail(err)
	}
	if scope == "global" && projectPath != "" {
		return fail(apperr.Usage("invalid_flag_combination", "--path cannot be used with --global"))
	}
	if scope == "" && projectPath != "" {
		scope = "project"
	}
	env.Runtime = profile.ID
	env.Scope = scope

	scopesToCheck := []string{scope}
	if scope == "" {
		scopesToCheck = []string{"project", "global"}
	}
	var all []installer.HookStatusInfo
	hasBrokenOrModified := false
	for _, sc := range scopesToCheck {
		roots, rootWarnings, err := resolveScopeRoots(ctx, deps, profile, sc, projectPath)
		if err != nil {
			if sc == "project" && scope == "" {
				var appErr *apperr.Error
				if errors.As(err, &appErr) && (appErr.Code == "home_as_project" || appErr.Code == "filesystem_root") {
					env.Warnings = append(env.Warnings, fmt.Sprintf("project hooks check skipped: %s", appErr.Message))
					continue
				}
			}
			return fail(err)
		}
		env.Warnings = append(env.Warnings, rootWarnings...)
		items, err := installer.InspectHooksForRuntime(profile.ID, sc, roots)
		if err != nil {
			return fail(err)
		}
		for _, item := range items {
			if item.Status == "broken_executable" || item.Status == "modified" || item.Status == "missing" || item.Status == "conflicted" {
				hasBrokenOrModified = true
			}
		}
		all = append(all, items...)
	}
	env.Hooks = all
	if jsonOutput {
		if sub == "status" && hasBrokenOrModified {
			return failJSON(deps.Stdout, env, apperr.Conflict("hook_drift", "hook configuration drift or broken executable detected"))
		}
		return writeEnvelope(deps.Stdout, env)
	}
	for _, h := range all {
		fmt.Fprintf(deps.Stdout, "%s\t%s\t%s\t%s\n", h.Scope, h.ID, h.Event, h.Status)
		if h.Detail != "" {
			fmt.Fprintf(deps.Stdout, "  detail: %s\n", h.Detail)
		}
		if h.Hint != "" {
			fmt.Fprintf(deps.Stdout, "  hint: %s\n", h.Hint)
		}
	}
	if sub == "status" && hasBrokenOrModified {
		return apperr.Conflict("hook_drift", "hook configuration drift or broken executable detected")
	}
	return nil
}

func runHooksEnable(ctx context.Context, args []string, deps Deps, env Envelope, jsonOutput bool, fail func(error) error) error {
	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		return fail(apperr.Usage("missing_hook", "usage: dtkit hooks enable <id> --project|--global [--path <dir>] [--executable <abs>] [--dry-run] [--yes] [--json]"))
	}
	hookID := args[0]
	var scope, projectPath, exeOverride, runtimeArg string
	var dryRun, yes bool
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose exactly one of --project or --global"))
			}
			scope = "project"
		case "--global":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose exactly one of --project or --global"))
			}
			scope = "global"
		case "--dry-run":
			dryRun = true
		case "--yes":
			yes = true
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--runtime":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--runtime requires a value"))
			}
			runtimeArg = args[i]
		case "--path", "--executable":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			if flag == "--path" {
				projectPath = args[i]
			} else {
				exeOverride = args[i]
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown hooks enable option %q", args[i])))
		}
	}
	profile, err := checkHooksRuntime(runtimeArg)
	if err != nil {
		return fail(err)
	}
	env.DryRun = dryRun
	env.Runtime = profile.ID
	env.Scope = scope

	if scope == "" {
		return fail(apperr.WithHint(apperr.Usage("missing_scope", "choose --project or --global"), "example: dtkit hooks enable session-context --project --dry-run"))
	}
	if scope == "global" && projectPath != "" {
		return fail(apperr.Usage("invalid_flag_combination", "--path cannot be used with --global"))
	}

	roots, otherRoots, warnings, err := resolveHookAndSiblingRoots(ctx, deps, profile, scope, projectPath)
	if err != nil {
		return fail(err)
	}
	env.Warnings = append(env.Warnings, warnings...)
	env.Roots = roots

	stableExe, err := hooks.ResolveStableExecutable(exeOverride, deps.Executable, deps.LookPath)
	if err != nil {
		return fail(err)
	}

	preview, changed, err := installer.EnableHookForRuntime(profile.ID, scope, hookID, roots, otherRoots, stableExe, true, deps.Now(), nil)
	if err != nil {
		return fail(err)
	}
	env.Hooks = []installer.HookStatusInfo{preview}
	if !changed {
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Hook %s (%s) is already enabled (%s).\n", preview.ID, scope, preview.Status)
		return nil
	}
	if dryRun {
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Read-only hook enable preview: %s (%s) -> %s:%s\n", preview.ID, scope, preview.Root, preview.Path)
		fmt.Fprintf(deps.Stdout, "  command: %s\n  commandWindows: %s\n", preview.Command, preview.CommandWindows)
		return nil
	}

	if jsonOutput && !yes {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "--json mutation requires --yes"), "pass --dry-run to preview or --yes to apply"))
	}
	if !yes && !deps.IsInteractive() {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "non-interactive hooks enable requires --yes; run --dry-run first"), "run with --dry-run first, then pass --yes"))
	}
	if !yes {
		confirmed, err := confirmPrompt(ctx, deps, fmt.Sprintf("Register %s hook %s in %s (%s:%s)?", profile.DisplayName, hookID, scope, preview.Root, preview.Path))
		if err != nil {
			return fail(err)
		}
		if !confirmed {
			return fail(apperr.Cancelled("hook enable cancelled by user"))
		}
	}
	if ctx.Err() != nil {
		return fail(apperr.Cancelled("hook enable cancelled"))
	}

	applied, _, err := installer.EnableHookForRuntime(profile.ID, scope, hookID, roots, otherRoots, stableExe, false, deps.Now(), nil)
	if err != nil {
		return fail(err)
	}
	env.Hooks = []installer.HookStatusInfo{applied}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Enabled hook %s (%s): %s\n", applied.ID, scope, applied.Status)
	if applied.Hint != "" {
		fmt.Fprintf(deps.Stdout, "Next: %s\n", applied.Hint)
	}
	return nil
}

func runHooksDisable(ctx context.Context, args []string, deps Deps, env Envelope, jsonOutput bool, fail func(error) error) error {
	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		return fail(apperr.Usage("missing_hook", "usage: dtkit hooks disable <id> --project|--global [--path <dir>] [--dry-run] [--yes] [--json]"))
	}
	hookID := args[0]
	var scope, projectPath, runtimeArg string
	var dryRun, yes bool
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose exactly one of --project or --global"))
			}
			scope = "project"
		case "--global":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose exactly one of --project or --global"))
			}
			scope = "global"
		case "--dry-run":
			dryRun = true
		case "--yes":
			yes = true
		case "--json":
			jsonOutput = true
		case "--verbose":
			deps.Verbose = true
		case "--runtime":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--runtime requires a value"))
			}
			runtimeArg = args[i]
		case "--path":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a value"))
			}
			projectPath = args[i]
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown hooks disable option %q", args[i])))
		}
	}
	profile, err := checkHooksRuntime(runtimeArg)
	if err != nil {
		return fail(err)
	}
	env.DryRun = dryRun
	env.Runtime = profile.ID
	env.Scope = scope

	if scope == "" {
		return fail(apperr.WithHint(apperr.Usage("missing_scope", "choose --project or --global"), "example: dtkit hooks disable session-context --project --dry-run"))
	}
	if scope == "global" && projectPath != "" {
		return fail(apperr.Usage("invalid_flag_combination", "--path cannot be used with --global"))
	}

	roots, _, warnings, err := resolveHookAndSiblingRoots(ctx, deps, profile, scope, projectPath)
	if err != nil {
		return fail(err)
	}
	env.Warnings = append(env.Warnings, warnings...)
	env.Roots = roots

	preview, changed, err := installer.DisableHookForRuntime(profile.ID, scope, hookID, roots, true, deps.Now(), nil)
	if err != nil {
		return fail(err)
	}
	env.Hooks = []installer.HookStatusInfo{preview}
	if !changed {
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Hook %s (%s) is already disabled.\n", hookID, scope)
		return nil
	}
	if dryRun {
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Read-only hook disable preview: %s (%s) -> %s:%s\n", hookID, scope, preview.Root, preview.Path)
		return nil
	}
	if jsonOutput && !yes {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "--json mutation requires --yes"), "pass --dry-run to preview or --yes to apply"))
	}
	if !yes && !deps.IsInteractive() {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "non-interactive hooks disable requires --yes; run --dry-run first"), "run with --dry-run first, then pass --yes"))
	}
	if !yes {
		confirmed, err := confirmPrompt(ctx, deps, fmt.Sprintf("Disable %s hook %s in %s?", profile.DisplayName, hookID, scope))
		if err != nil {
			return fail(err)
		}
		if !confirmed {
			return fail(apperr.Cancelled("hook disable cancelled by user"))
		}
	}
	if ctx.Err() != nil {
		return fail(apperr.Cancelled("hook disable cancelled"))
	}

	applied, _, err := installer.DisableHookForRuntime(profile.ID, scope, hookID, roots, false, deps.Now(), nil)
	if err != nil {
		return fail(err)
	}
	env.Hooks = []installer.HookStatusInfo{applied}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Disabled hook %s (%s).\n", applied.ID, scope)
	return nil
}

func resolveHookAndSiblingRoots(ctx context.Context, deps Deps, profile adapter.RuntimeProfile, scope, projectPath string) (roots map[string]string, sibling map[string]string, warnings []string, err error) {
	roots, warnings, err = resolveScopeRoots(ctx, deps, profile, scope, projectPath)
	if err != nil {
		return nil, nil, nil, err
	}
	otherScope := "global"
	if scope == "global" {
		otherScope = "project"
	}
	if other, _, oErr := resolveScopeRoots(ctx, deps, profile, otherScope, ""); oErr == nil {
		sibling = other
	}
	return roots, sibling, warnings, nil
}

func checkHooksRuntime(runtimeArg string) (adapter.RuntimeProfile, error) {
	profile, err := resolveRuntimeProfile(runtimeArg)
	if err != nil {
		return profile, err
	}
	if !profile.HooksSupported {
		return profile, apperr.WithHint(
			apperr.Usage("hooks_not_supported", fmt.Sprintf("DTKit does not manage hooks for %s", profile.DisplayName)),
			"DTKit hooks currently support Codex CLI (--runtime codex) and Claude Code (--runtime claude)",
		)
	}
	return profile, nil
}

func runHookExec(ctx context.Context, args []string, deps Deps) error {
	if len(args) < 2 || args[0] != "run" {
		return apperr.Usage("invalid_hook_usage", "usage: dtkit hook run <id> --scope project|global --root <dir> [--runtime <codex|claude>]")
	}
	hookID := args[1]
	var scope, rootDir, runtimeArg string
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--scope":
			i++
			if i >= len(args) {
				return apperr.Usage("missing_flag_value", "--scope requires a value")
			}
			scope = args[i]
		case "--root":
			i++
			if i >= len(args) {
				return apperr.Usage("missing_flag_value", "--root requires a value")
			}
			rootDir = args[i]
		case "--runtime":
			i++
			if i >= len(args) {
				return apperr.Usage("missing_flag_value", "--runtime requires a value")
			}
			runtimeArg = args[i]
		default:
			return apperr.Usage("unknown_option", fmt.Sprintf("unknown hook run option %q", args[i]))
		}
	}
	if _, err := hooks.ResolveHookSpec(runtimeArg, hookID); err != nil {
		return err
	}
	switch hookID {
	case "session-context":
		return hooks.RunSessionContext(ctx, deps.Stdin, deps.Stdout, deps.Stderr, scope, rootDir)
	case "check-changed":
		return hooks.RunCheckChanged(ctx, deps.Stdin, deps.Stdout, deps.Stderr, scope, rootDir)
	case "guard-edit":
		return hooks.RunGuardEdit(ctx, deps.Stdin, deps.Stdout, deps.Stderr, scope, rootDir)
	default:
		return apperr.Usage("unsupported_hook", fmt.Sprintf("unsupported hook handler %q", hookID))
	}
}
