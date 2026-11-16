package cli

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/apperr"
	"dtkit.local/dtkit/internal/catalog"
	"dtkit.local/dtkit/internal/installer"
	"dtkit.local/dtkit/internal/paths"
	"dtkit.local/dtkit/internal/planner"
	"dtkit.local/dtkit/internal/state"
)

func runInstall(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("install")
	jsonOutput := hasJSONFlag(args)
	fail := func(err error) error {
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	if len(args) == 0 {
		return fail(apperr.Usage("missing_kit", "usage: dtkit install <kit> --project|--global [--path <dir>] [--from <dir>] [--dry-run] [--yes] [--json]"))
	}
	kitID := args[0]
	if kitID == "" || kitID[0] == '-' {
		return fail(apperr.Usage("missing_kit", "usage: dtkit install <kit> --project|--global [--path <dir>] [--from <dir>] [--dry-run] [--yes] [--json]"))
	}

	var scope, projectPath, runtimeArg, fromDir string
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
		case "--runtime", "--path", "--from":
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
				fromDir = args[i]
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown install option %q", args[i])))
		}
	}

	env.DryRun = dryRun
	env.Runtime = "codex"
	env.Scope = scope

	if scope == "" {
		return fail(apperr.WithHint(apperr.Usage("missing_scope", "choose --project or --global"), "example: dtkit install engineer --project --dry-run"))
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

	var pkg catalog.Package
	var source fs.FS
	var pkgDigest string
	sourceKind := "bundled"
	var err error
	if fromDir != "" {
		sourceKind = "local"
		pkg, source, pkgDigest, err = catalog.LoadDir(fromDir)
		if err == nil && pkg.ID != kitID {
			err = apperr.Usage("kit_id_mismatch", fmt.Sprintf("local package ID %q does not match requested %q", pkg.ID, kitID))
		}
	} else {
		if kitID != "engineer" {
			err = apperr.Usage("unknown_kit", fmt.Sprintf("kit %q is not in the bundled catalog", kitID))
		} else {
			pkg, source, err = catalog.Engineer()
			if err == nil {
				pkgDigest, err = catalog.Digest(source)
			}
		}
	}
	if err != nil {
		return fail(err)
	}
	env.Kit = &KitSummary{ID: pkg.ID, Version: pkg.Version, Digest: pkgDigest}

	artifacts, err := codex.Render(pkg, source, scope)
	if err != nil {
		return fail(err)
	}

	pending, lockStatus, err := installer.InspectTransactions(scope, roots)
	if err != nil {
		return fail(err)
	}
	if len(pending) > 0 || lockStatus.Present {
		return fail(apperr.WithHint(apperr.Conflict("recovery_required", "unfinished transaction or lock exists"), fmt.Sprintf("run dtkit recover --%s --dry-run", scope)))
	}

	statePath, err := paths.ResolveTarget(roots[scopeRoot(scope)], ".dtkit/state.json")
	if err != nil {
		return fail(err)
	}
	existing, stateExists, err := state.LoadFile(statePath, scope)
	if err != nil {
		return fail(err)
	}

	if stateExists {
		if existing.Kit.ID != pkg.ID {
			return fail(apperr.WithHint(apperr.Conflict("different_kit_installed", fmt.Sprintf("kit %q is already installed in %s scope", existing.Kit.ID, scope)), fmt.Sprintf("run dtkit remove %s --%s first", existing.Kit.ID, scope)))
		}
		if scope == "global" && !sameAbsPath(existing.CodexHome, roots["codex-home"]) {
			return fail(apperr.Conflict("profile_mismatch", fmt.Sprintf("installed Codex home %q does not match current %q", existing.CodexHome, roots["codex-home"])))
		}
		if existing.Kit.Version != pkg.Version || existing.Kit.Digest != pkgDigest || existing.AdapterRevision != codex.AdapterRevision {
			return fail(apperr.WithHint(apperr.Conflict("already_installed", fmt.Sprintf("%s %s is already installed with different version or content", existing.Kit.ID, existing.Kit.Version)), fmt.Sprintf("run dtkit update %s --%s", pkg.ID, scope)))
		}
		plan, err := planner.PlanDesired(scope, pkg.ID, roots, existing, artifacts)
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

	plan, err := planner.PlanDesired(scope, pkg.ID, roots, nil, artifacts)
	if err != nil {
		return fail(err)
	}
	env.Actions, env.Summary = summarizeActions(plan.Actions)

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

	if _, lookErr := deps.LookPath("codex"); lookErr != nil {
		env.Warnings = append(env.Warnings, "codex executable not found on PATH; files will still be installed")
	}

	installed, err := installer.FreshInstallWithOptions(scope, roots, pkg, source, artifacts, deps.Now(), installer.InstallOptions{
		SourceKind: sourceKind,
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
	fmt.Fprintf(deps.Stdout, "Installed %s %s (%s) for Codex (%d artifacts).\n", pkg.ID, pkg.Version, scope, len(installed.Artifacts))
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
