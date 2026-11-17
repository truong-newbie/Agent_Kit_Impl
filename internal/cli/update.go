package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
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
		return fail(apperr.Usage("missing_kit", "usage: dtkit update <kit>[@<version>] --project|--global [--runtime <codex|gemini|opencode|cursor|claude>] [--preset <name>|--components <list>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--yes] [--json]"))
	}
	kitSpec := args[0]

	var scope, projectPath, runtimeArg, fromPath, sourceURL, sha256Hex, presetRaw, componentsRaw string
	var dryRun, yes, offline, hasPreset, hasComponents bool
	for i := 1; i < len(args); i++ {
		switch {
		case args[i] == "--project":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose exactly one of --project or --global"))
			}
			scope = "project"
		case args[i] == "--global":
			if scope != "" {
				return fail(apperr.Usage("conflicting_scope", "choose exactly one of --project or --global"))
			}
			scope = "global"
		case args[i] == "--dry-run":
			dryRun = true
		case args[i] == "--json":
			jsonOutput = true
		case args[i] == "--yes":
			yes = true
		case args[i] == "--offline":
			offline = true
		case args[i] == "--verbose":
			deps.Verbose = true
		case strings.HasPrefix(args[i], "--preset="):
			hasPreset = true
			presetRaw = strings.TrimPrefix(args[i], "--preset=")
		case strings.HasPrefix(args[i], "--components="):
			hasComponents = true
			componentsRaw = strings.TrimPrefix(args[i], "--components=")
		case args[i] == "--runtime" || args[i] == "--path" || args[i] == "--from" || args[i] == "--source" || args[i] == "--sha256" || args[i] == "--preset" || args[i] == "--components":
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
			case "--preset":
				hasPreset = true
				presetRaw = args[i]
			case "--components":
				hasComponents = true
				componentsRaw = args[i]
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown update option %q", args[i])))
		}
	}

	preset, requestedComponents, selErr := parseSelectionInputs(presetRaw, hasPreset, componentsRaw, hasComponents)
	if selErr != nil {
		return fail(selErr)
	}

	profile, rtErr := resolveRuntimeProfile(runtimeArg)
	if rtErr != nil {
		return fail(rtErr)
	}
	env.DryRun = dryRun
	env.Runtime = profile.ID
	env.Scope = scope

	if scope == "" {
		return fail(apperr.WithHint(apperr.Usage("missing_scope", "choose --project or --global"), "example: dtkit update engineer --project --dry-run"))
	}
	if scope == "global" && projectPath != "" {
		return fail(apperr.Usage("invalid_flag_combination", "--path cannot be used with --global"))
	}

	roots, rootWarnings, err := resolveScopeRoots(ctx, deps, profile, scope, projectPath)
	if err != nil {
		return fail(err)
	}
	env.Warnings = append(env.Warnings, rootWarnings...)
	env.Roots = roots

	resolved, err := resolveKitSource(ctx, deps, kitSpec, fromPath, sourceURL, sha256Hex, offline, dist.CacheReadOnly)
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
	if err := applyScopeCapabilitiesAndBinaryWarnings(deps, profile, scope, &pkg, &env); err != nil {
		return fail(err)
	}

	if !hasPreset && !hasComponents {
		statePath, spErr := resolveRuntimeStatePath(profile, scope, roots)
		if spErr != nil {
			return fail(spErr)
		}
		existingState, stateExists, stErr := state.LoadFileForRuntime(statePath, scope, profile.ID)
		if stErr != nil {
			return fail(stErr)
		}
		if stateExists && existingState.Selection != nil {
			preset = existingState.Selection.Preset
			requestedComponents = existingState.Selection.RequestedComponents
		}
	}

	selectionSpec, artifacts, err := renderWithSelection(profile.ID, pkg, source, scope, preset, requestedComponents)
	if err != nil {
		return fail(err)
	}
	env.Selection = selectionSpec

	plan, existing, adapterMigration, err := installer.PlanUpdateForRuntime(profile.ID, scope, roots, pkg, pkgDigest, artifacts, profile.Revision)
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
	if !hasMutation && existing.Kit.Version == pkg.Version && existing.Kit.Digest == pkgDigest && !adapterMigration && catalog.SelectionsEqual(existing.Selection, selectionSpec) {
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

	if err := persistResolvedCache(deps.Env, resolved); err != nil {
		return fail(err)
	}

	res, err := installer.Update(scope, roots, pkg, source, artifacts, deps.Now(), installer.UpdateOptions{
		Runtime:       profile.ID,
		SourceKind:    resolved.SourceKind,
		ArchiveSHA256: resolved.ArchiveSHA256,
		Selection:     selectionSpec,
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
