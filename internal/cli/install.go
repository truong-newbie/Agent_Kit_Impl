package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func runInstall(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("install")
	jsonOutput := hasJSONFlag(args)
	redact := false
	fail := func(err error) error {
		if redact {
			err = redactAppError(err, collectHomeCandidates(deps))
		}
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 || args[0] == "" || args[0][0] == '-' {
		return fail(apperr.Usage("missing_kit", "usage: dtkit install <kit>[@<ver>] --project|--global [--runtime <codex|gemini|opencode|cursor|claude>] [--preset <name>|--components <list>] [--path <dir>] [--from <dir|archive>] [--source <url>] [--sha256 <hex>] [--offline] [--dry-run] [--diff] [--redact] [--yes] [--json]"))
	}
	kitSpec := args[0]

	var scope, projectPath, runtimeArg, fromPath, sourceURL, sha256Hex, presetRaw, componentsRaw string
	var dryRun, yes, offline, showDiff, hasPreset, hasComponents bool
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
		case args[i] == "--diff":
			showDiff = true
		case args[i] == "--redact":
			redact = true
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
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown install option %q", args[i])))
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
		return fail(apperr.WithHint(apperr.Usage("missing_scope", "choose --project or --global"), "example: dtkit install engineer --project --dry-run"))
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

	selectionSpec, artifacts, err := renderWithSelection(profile.ID, pkg, source, scope, preset, requestedComponents)
	if err != nil {
		return fail(err)
	}
	env.Selection = selectionSpec

	pending, lockStatus, err := installer.InspectTransactionsForRuntime(profile.ID, scope, roots)
	if err != nil {
		return fail(err)
	}
	if len(pending) > 0 || lockStatus.Present {
		return fail(apperr.WithHint(apperr.Conflict("recovery_required", "unfinished transaction or lock exists"), fmt.Sprintf("run dtkit recover --%s --runtime %s --dry-run", scope, profile.ID)))
	}

	statePath, err := resolveRuntimeStatePath(profile, scope, roots)
	if err != nil {
		return fail(err)
	}
	reg, regExists, err := state.LoadRegistryForRuntime(statePath, scope, profile.ID)
	if err != nil {
		return fail(err)
	}

	var peerWhole map[string]string
	if regExists {
		sole := reg.Kits[reg.KitIDs()[0]]
		if err := checkRuntimeHomeBinding(profile, scope, &sole, roots); err != nil {
			return fail(err)
		}
		if existing, kitInstalled := reg.Kits[pkg.ID]; kitInstalled {
			if existing.Kit.Version != pkg.Version || existing.Kit.Digest != pkgDigest || existing.AdapterRevision != profile.Revision || !catalog.SelectionsEqual(existing.Selection, selectionSpec) {
				return fail(apperr.WithHint(apperr.Conflict("already_installed", fmt.Sprintf("%s %s is already installed with different version, selection, or content", existing.Kit.ID, existing.Kit.Version)), fmt.Sprintf("run dtkit update %s --%s --runtime %s", pkg.ID, scope, profile.ID)))
			}
			plan, err := planner.PlanDesiredForRuntime(profile.ID, scope, pkg.ID, roots, &existing, artifacts)
			if err != nil {
				return fail(err)
			}
			env.Actions, env.Summary = summarizeActions(plan.Actions)
			if plan.Conflicts != 0 {
				return fail(apperr.Conflict("installation_drift", fmt.Sprintf("existing installation has %d conflict(s) or drifted artifact(s)", plan.Conflicts)))
			}
			installedFlag := true
			env.Installed = &installedFlag
			env.Generation = existing.Generation
			if jsonOutput {
				return writeEnvelope(deps.Stdout, env)
			}
			for _, w := range env.Warnings {
				fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
			}
			if dryRun {
				fmt.Fprintf(deps.Stdout, "Read-only preview: %s %s (%s) is already up to date (%d noop).\n", pkg.ID, scope, roots[scopeRoot(scope)], len(plan.Actions))
			} else {
				fmt.Fprintf(deps.Stdout, "Already installed and up to date: %s %s (%s, generation %d).\n", pkg.ID, pkg.Version, scope, existing.Generation)
			}
			return nil
		}
		peerWhole = reg.PeerOwnedWholeFiles(pkg.ID)
	}

	plan, err := planner.PlanDesiredWithPeersForRuntime(profile.ID, scope, pkg.ID, roots, nil, peerWhole, artifacts)
	if err != nil {
		return fail(err)
	}
	env.Actions, env.Summary = summarizeActions(plan.Actions)
	if showDiff {
		diff, dErr := planner.ComputePlanDiff(profile.ID, scope, pkg.ID, roots, plan, artifacts, false)
		if dErr != nil {
			return fail(dErr)
		}
		explain := buildExplainView(&resolved, fromPath, sourceURL, nil, selectionSpec, plan, artifacts)
		env.Explain = &explain
		env.Diff = &diff
	}
	if redact {
		env.Roots = cloneRootsMap(roots)
		redactEnvelope(&env, collectHomeCandidates(deps))
	}

	if plan.Conflicts != 0 {
		return fail(apperr.Conflict("install_conflicts", fmt.Sprintf("install found %d conflict(s)", plan.Conflicts)))
	}

	if dryRun {
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		for _, w := range env.Warnings {
			fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
		}
		fmt.Fprintf(deps.Stdout, "Read-only preview: %s %s (%s)\n", pkg.ID, scope, roots[scopeRoot(scope)])
		for _, action := range plan.Actions {
			fmt.Fprintf(deps.Stdout, "%s\t%s:%s\t%s\n", action.Kind, action.Root, action.Path, action.Reason)
		}
		if showDiff {
			fmt.Fprintln(deps.Stdout)
			printPlanDiffText(deps.Stdout, env.Explain, env.Diff)
		}
		fmt.Fprintf(deps.Stdout, "Conflicts: %d. Nothing was written.\n", plan.Conflicts)
		return nil
	}

	if jsonOutput && !yes {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "--json mutation requires --yes"), "pass --dry-run to preview or --yes to apply"))
	}
	if !yes && !deps.IsInteractive() {
		return fail(apperr.WithHint(apperr.Usage("confirmation_required", "non-interactive install requires --yes; run --dry-run first"), "run with --dry-run to inspect the plan, then pass --yes"))
	}

	if !yes {
		fmt.Fprintf(deps.Stderr, "Planned install: %s %s (%s, %d artifacts)\n", pkg.ID, pkg.Version, scope, len(plan.Actions))
		for _, action := range plan.Actions {
			fmt.Fprintf(deps.Stderr, "  %s\t%s:%s\n", action.Kind, action.Root, action.Path)
		}
		confirmed, err := confirmPrompt(ctx, deps, "Proceed with installation?")
		if err != nil {
			return fail(err)
		}
		if !confirmed {
			return fail(apperr.Cancelled("installation cancelled by user"))
		}
	}

	if ctx.Err() != nil {
		return fail(apperr.Cancelled("installation cancelled"))
	}

	if err := persistResolvedCache(deps.Env, resolved); err != nil {
		return fail(err)
	}

	installed, err := installer.FreshInstallWithOptions(scope, roots, pkg, source, artifacts, deps.Now(), installer.InstallOptions{
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
	env.Generation = installed.Generation

	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	for _, w := range env.Warnings {
		fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
	}
	fmt.Fprintf(deps.Stdout, "Installed %s %s (%s) for %s (%d artifacts).\n", pkg.ID, pkg.Version, scope, profile.DisplayName, len(installed.Artifacts))
	if scope == "project" {
		fmt.Fprintf(deps.Stdout, "Tip: add .dtkit/local/ to .gitignore so local transaction backups are not committed.\n")
	}
	return nil
}

func scopeRoot(scope string) string {
	if scope == "global" {
		return "user-home"
	}
	return "project"
}

func sameAbsPath(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}
