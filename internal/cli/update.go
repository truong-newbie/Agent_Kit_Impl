package cli

import (
	"context"
	"fmt"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

func runUpdate(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("update")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		return fail(apperr.Usage("missing_kit", "usage: dtkit update <kit>[@<version>] --project|--global [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--yes] [--json]"))
	}
	kitSpec := args[0]

	var scope, projectPath, runtimeArg, fromPath, sourceURL, sha256Hex string
	var dryRun, yes, offline bool
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
		case "--offline":
			offline = true
		case "--verbose":
			deps.Verbose = true
		case "--runtime", "--path", "--from", "--source", "--sha256":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--runtime":
				runtimeArg = args[i]
			case "--path":
				projectPath = args[i]
			case "--from":
				fromPath = args[i]
			case "--source":
				sourceURL = args[i]
			case "--sha256":
				sha256Hex = args[i]
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown update option %q", args[i])))
		}
	}

	env.DryRun = dryRun
	env.Runtime = "codex"
	env.Scope = scope

	if scope == "" {
		return fail(apperr.WithHint(apperr.Usage("missing_scope", "choose --project or --global"), "example: dtkit update engineer --project --dry-run"))
	}
	if runtimeArg != "" && runtimeArg != "codex" {
		return fail(apperr.Usage("unsupported_runtime", fmt.Sprintf("unsupported runtime %q", runtimeArg)))
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

	resolved, err := resolveKitSource(ctx, deps, kitSpec, fromPath, sourceURL, sha256Hex, offline)
	if err != nil {
		return fail(err)
	}
	pkg := resolved.Package
	source := resolved.Source
	pkgDigest := resolved.PackageDigest
	env.Kit = &KitSummary{
		ID:            pkg.ID,
		Version:       pkg.Version,
		Digest:        pkgDigest,
		ArchiveSHA256: resolved.ArchiveSHA256,
	}

	artifacts, err := codex.Render(pkg, source, scope)
	if err != nil {
		return fail(err)
	}

	plan, existing, adapterMigration, err := installer.PlanUpdate(scope, roots, pkg, pkgDigest, artifacts, codex.AdapterRevision)
	if err != nil {
		env.Actions, env.Summary = summarizeActions(plan.Actions)
		return fail(err)
	}
	env.Actions, env.Summary = summarizeActions(plan.Actions)

	if plan.Conflicts != 0 {
		return fail(apperr.Conflict("update_conflicts", fmt.Sprintf("update found %d conflict(s)", plan.Conflicts)))
	}

	hasMutation := false
	for _, a := range plan.Actions {
		if a.Kind != "noop" {
			hasMutation = true
			break
		}
	}
	if !hasMutation && existing.Kit.Version == pkg.Version && existing.Kit.Digest == pkgDigest && !adapterMigration {
		installedFlag := true
		env.Installed = &installedFlag
		env.Generation = existing.Generation
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		for _, w := range env.Warnings {
			fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
		}
		fmt.Fprintf(deps.Stdout, "Already up to date: %s %s (%s, generation %d).\n", pkg.ID, pkg.Version, scope, existing.Generation)
		return nil
	}

	if dryRun {
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		for _, w := range env.Warnings {
			fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
		}
		label := "update"
		if adapterMigration {
			label = "adapter migration"
		}
		fmt.Fprintf(deps.Stdout, "Read-only %s preview: %s %s -> %s (%s)\n", label, pkg.ID, existing.Kit.Version, pkg.Version, scope)
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
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "non-interactive update requires --yes; run --dry-run first"), "run with --dry-run to inspect the plan, then pass --yes"))
	}
	if !yes {
		fmt.Fprintf(deps.Stderr, "Planned update: %s %s -> %s (%s)\n", pkg.ID, existing.Kit.Version, pkg.Version, scope)
		confirmed, err := confirmPrompt(ctx, deps, "Proceed with update?")
		if err != nil {
			return fail(err)
		}
		if !confirmed {
			return fail(apperr.Cancelled("update cancelled by user"))
		}
	}

	if ctx.Err() != nil {
		return fail(apperr.Cancelled("update cancelled"))
	}

	res, err := installer.Update(scope, roots, pkg, source, artifacts, deps.Now(), installer.UpdateOptions{
		SourceKind:    resolved.SourceKind,
		ArchiveSHA256: resolved.ArchiveSHA256,
	})
	if err != nil {
		return fail(err)
	}
	installedFlag := true
	env.Installed = &installedFlag
	env.Generation = res.Installation.Generation
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	for _, w := range env.Warnings {
		fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
	}
	fmt.Fprintf(deps.Stdout, "Updated %s %s -> %s (%s, generation %d).\n", pkg.ID, existing.Kit.Version, pkg.Version, scope, res.Installation.Generation)
	return nil
}
