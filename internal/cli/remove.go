package cli

import (
	"context"
	"fmt"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func runRemove(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("remove")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		return fail(apperr.Usage("missing_kit", "usage: dtkit remove <kit> --project|--global [--path <dir>] [--dry-run] [--yes] [--json]"))
	}
	kitID := args[0]

	var scope, projectPath string
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
		case "--json":
			jsonOutput = true
		case "--yes":
			yes = true
		case "--verbose":
			deps.Verbose = true
		case "--path":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a value"))
			}
			projectPath = args[i]
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown remove option %q", args[i])))
		}
	}

	env.DryRun = dryRun
	env.Runtime = "codex"
	env.Scope = scope

	if scope == "" {
		return fail(apperr.WithHint(apperr.Usage("missing_scope", "choose --project or --global"), "example: dtkit remove engineer --project --dry-run"))
	}
	if scope == "global" && projectPath != "" {
		return fail(apperr.Usage("invalid_flag_combination", "--path cannot be used with --global"))
	}

	roots := map[string]string{}
	if scope == "project" {
		res, err := paths.ResolveProject(ctx, deps.Env, projectPath)
		if err != nil {
			return fail(err)
		}
		roots["project"] = res.Root
		env.Warnings = append(env.Warnings, res.Warnings...)
	} else {
		res, err := paths.ResolveGlobal(deps.Env)
		if err != nil {
			return fail(err)
		}
		roots["user-home"] = res.UserHome
		roots["codex-home"] = res.CodexHome
		env.Warnings = append(env.Warnings, res.Warnings...)
	}
	env.Roots = roots

	plan, existing, exists, err := installer.PlanRemove(scope, kitID, roots)
	if existing != nil {
		env.Kit = &KitSummary{ID: existing.Kit.ID, Version: existing.Kit.Version, Digest: existing.Kit.Digest}
	} else {
		env.Kit = &KitSummary{ID: kitID, Version: ""}
	}
	if err != nil {
		env.Actions, env.Summary = summarizeActions(plan.Actions)
		return fail(err)
	}
	env.Actions, env.Summary = summarizeActions(plan.Actions)

	if !exists {
		installedFlag := false
		env.Installed = &installedFlag
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "DTKit is not installed for %s (nothing to remove).\n", scope)
		return nil
	}

	if plan.Conflicts != 0 {
		return fail(apperr.Conflict("remove_conflicts", fmt.Sprintf("remove found %d conflict(s)", plan.Conflicts)))
	}

	if dryRun {
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		for _, w := range env.Warnings {
			fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
		}
		fmt.Fprintf(deps.Stdout, "Read-only remove preview: %s %s (%s)\n", existing.Kit.ID, scope, roots[scopeRoot(scope)])
		for _, action := range plan.Actions {
			fmt.Fprintf(deps.Stdout, "%s\t%s:%s\t%s\n", action.Kind, action.Root, action.Path, action.Reason)
		}
		fmt.Fprintf(deps.Stdout, "Conflicts: %d. Nothing was written.\n", plan.Conflicts)
		return nil
	}

	if jsonOutput && !yes {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "--json mutation requires --yes"), "pass --dry-run to preview or --yes to apply"))
	}
	if !yes && !deps.IsInteractive() {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "non-interactive remove requires --yes; run --dry-run first"), "run with --dry-run to inspect the plan, then pass --yes"))
	}
	if !yes {
		fmt.Fprintf(deps.Stderr, "Planned remove: %s %s (%s, %d actions)\n", existing.Kit.ID, existing.Kit.Version, scope, len(plan.Actions))
		confirmed, err := confirmPrompt(ctx, deps, "Proceed with removal?")
		if err != nil {
			return fail(err)
		}
		if !confirmed {
			return fail(apperr.Cancelled("removal cancelled by user"))
		}
	}

	if ctx.Err() != nil {
		return fail(apperr.Cancelled("removal cancelled"))
	}

	res, err := installer.Remove(scope, kitID, roots, deps.Now(), installer.RemoveOptions{})
	if err != nil {
		return fail(err)
	}
	installedFlag := false
	env.Installed = &installedFlag
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	for _, w := range env.Warnings {
		fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
	}
	fmt.Fprintf(deps.Stdout, "Removed %s %s (%s, %d actions).\n", existing.Kit.ID, existing.Kit.Version, scope, len(res.Plan.Actions))
	return nil
}
