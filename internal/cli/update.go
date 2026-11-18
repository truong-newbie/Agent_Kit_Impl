package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/scout"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func runRefresh(ctx context.Context, args []string, deps Deps) error {
	return runUpdateProject(ctx, args, deps, "refresh")
}

func runUpdate(ctx context.Context, args []string, deps Deps) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runUpdateProject(ctx, args, deps, "update")
	}

	env := newEnvelope("update")
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
	if applied, oErr := applyProjectOverridesToResolved(scope, roots, &resolved); oErr != nil {
		return fail(oErr)
	} else if len(applied) > 0 {
		env.Warnings = append(env.Warnings, fmt.Sprintf("applied %d project override(s) from .dtkit/overrides/%s", len(applied), resolved.Package.ID))
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
		existingReg, regExists, stErr := state.LoadRegistryForRuntime(statePath, scope, profile.ID)
		if stErr != nil {
			return fail(stErr)
		}
		if regExists {
			if existingState, ok := existingReg.Kits[pkg.ID]; ok && existingState.Selection != nil {
				preset = existingState.Selection.Preset
				requestedComponents = existingState.Selection.RequestedComponents
			}
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
	if showDiff {
		diff, dErr := planner.ComputePlanDiff(profile.ID, scope, pkg.ID, roots, plan, artifacts, false)
		if dErr != nil {
			return fail(dErr)
		}
		explain := buildExplainView(&resolved, fromPath, sourceURL, existing, selectionSpec, plan, artifacts)
		env.Explain = &explain
		env.Diff = &diff
	}
	if redact {
		env.Roots = cloneRootsMap(roots)
		redactEnvelope(&env, collectHomeCandidates(deps))
	}

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

func runUpdateProject(ctx context.Context, args []string, deps Deps, cmdName string) error {
	env := newEnvelope(cmdName)
	jsonOutput := hasJSONFlag(args)
	redact := hasFlag(args, "--redact")
	fail := func(err error) error {
		if redact {
			err = redactAppError(err, collectHomeCandidates(deps))
		}
		if jsonOutput {
			return failJSON(deps.Stdout, env, err)
		}
		return err
	}

	var projectPath, runtimeArg string
	var dryRun, yes bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--path":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--path requires a directory"))
			}
			projectPath = args[i]
		case "--runtime":
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", "--runtime requires a runtime ID"))
			}
			runtimeArg = args[i]
		case "--dry-run":
			dryRun = true
		case "--yes", "-y":
			yes = true
		case "--json":
			jsonOutput = true
		case "--redact":
			redact = true
		case "--verbose":
			deps.Verbose = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown %s option %q", cmdName, args[i])))
			}
			return fail(apperr.Usage("invalid_argument", fmt.Sprintf("unexpected argument %q; run 'dtkit %s --path <dir> [--dry-run] [--yes]'", args[i], cmdName)))
		}
	}

	if !dryRun && !yes && (jsonOutput || !deps.IsInteractive()) {
		return fail(apperr.WithHint(
			apperr.Usage("confirmation_required", fmt.Sprintf("non-interactive %s requires --yes; run --dry-run first", cmdName)),
			fmt.Sprintf("run 'dtkit %s --dry-run' to preview, then pass --yes", cmdName),
		))
	}

	projRes, err := paths.ResolveProject(ctx, deps.Env, projectPath)
	if err != nil {
		return fail(err)
	}
	projRoot := projRes.Root
	env.Warnings = append(env.Warnings, projRes.Warnings...)
	env.DryRun = dryRun
	env.Scope = "project"

	var targetRuntimes []string
	if runtimeArg != "" {
		prof, rtErr := resolveRuntimeProfile(runtimeArg)
		if rtErr != nil {
			return fail(rtErr)
		}
		targetRuntimes = []string{prof.ID}
		env.Runtime = prof.ID
	} else {
		targetRuntimes = adapter.SupportedRuntimes()
	}

	if !dryRun && !yes && deps.IsInteractive() {
		confirmed, cErr := confirmPrompt(ctx, deps, fmt.Sprintf("Refresh codebase summary, project profile, installed kits, and lockfile at %s?", projRoot))
		if cErr != nil {
			return fail(cErr)
		}
		if !confirmed {
			return fail(apperr.Cancelled(fmt.Sprintf("%s cancelled by user", cmdName)))
		}
	}

	summary := UpdateSummary{
		Root: projRoot,
		Kits: []UpdateKitOutcome{},
	}

	// 1. Non-destructively sync .dtkit/project.json
	syncRes, pErr := project.SyncProfile(ctx, deps.Env, project.SyncOptions{Path: projRoot, DryRun: dryRun})
	if pErr != nil {
		return fail(pErr)
	}
	summary.ProjectProfile = syncRes.Updated
	summary.AddedRoots = syncRes.AddedRoots
	summary.NewSuggestedChecks = syncRes.NewSuggestedChecks
	if syncRes.NewSuggestedChecks > 0 {
		env.Warnings = append(env.Warnings, fmt.Sprintf("detected %d new quality check(s) via 'dtkit project inspect'; review before adding to .dtkit/project.json", syncRes.NewSuggestedChecks))
	}

	// 2. Sync/update installed kits across target runtimes
	kitOutcomes, kitWarnings, firstInstalledRT := syncProjectKits(ctx, deps, projRoot, targetRuntimes, dryRun)
	summary.Kits = kitOutcomes
	env.Warnings = append(env.Warnings, kitWarnings...)

	// 3. Refresh dtkit.lock if present
	lockUpdated, lockWarnings := syncProjectLockfile(ctx, deps, projRoot, firstInstalledRT, dryRun)
	summary.LockfileUpdated = lockUpdated
	env.Warnings = append(env.Warnings, lockWarnings...)

	// 4. Refresh docs/codebase-summary.md last so it captures updated project/kit/lockfile state cleanly
	sRep, sErr := scout.Run(ctx, deps.Env, scout.Options{Root: projRoot})
	if sErr != nil {
		return fail(sErr)
	}
	freshSummaryMD := scout.FormatMarkdownSummary(sRep)
	summaryTarget := filepath.Join(projRoot, "docs", "codebase-summary.md")
	oldSummaryBytes, readSumErr := os.ReadFile(summaryTarget)
	summaryChanged := readSumErr != nil || !bytes.Equal(oldSummaryBytes, []byte(freshSummaryMD))
	summary.CodebaseSummary = summaryChanged
	if !dryRun && summaryChanged {
		if _, wErr := scout.WriteCodebaseSummary(ctx, deps.Env, projRoot, freshSummaryMD); wErr != nil {
			return fail(wErr)
		}
		// Converge self-referential line count inside docs/codebase-summary.md so an immediate re-run is a clean no-op.
		if sRep2, sErr2 := scout.Run(ctx, deps.Env, scout.Options{Root: projRoot}); sErr2 == nil {
			if fresh2 := scout.FormatMarkdownSummary(sRep2); fresh2 != freshSummaryMD {
				_, _ = scout.WriteCodebaseSummary(ctx, deps.Env, projRoot, fresh2)
			}
		}
	}

	if redact {
		cands := collectHomeCandidates(deps)
		summary.Root = redactDiagnosticText(summary.Root, cands)
		for i := range env.Warnings {
			env.Warnings[i] = redactDiagnosticText(env.Warnings[i], cands)
		}
	}

	env.UpdateSummary = &summary
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	printUpdateSummaryText(deps, summary, dryRun)
	return nil
}

func syncProjectKits(ctx context.Context, deps Deps, projRoot string, targetRuntimes []string, dryRun bool) ([]UpdateKitOutcome, []string, string) {
	roots := map[string]string{"project": projRoot}
	outcomes := []UpdateKitOutcome{}
	var warnings []string
	firstInstalledRT := ""

	for _, rt := range targetRuntimes {
		reps, err := installer.InspectRuntimeAll(rt, "project", roots)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: unable to inspect installed kits: %v", rt, err))
			continue
		}
		prof, pErr := resolveRuntimeProfile(rt)
		if pErr != nil {
			continue
		}
		var regKits map[string]state.Installation
		if statePath, spErr := resolveRuntimeStatePath(prof, "project", roots); spErr == nil {
			if reg, exists, stErr := state.LoadRegistryForRuntime(statePath, "project", prof.ID); stErr == nil && exists {
				regKits = reg.Kits
			}
		}
		for _, rep := range reps {
			if !rep.Installed {
				continue
			}
			if firstInstalledRT == "" {
				firstInstalledRT = rt
			}
			sourceKind := ""
			if inst, ok := regKits[rep.KitID]; ok {
				sourceKind = inst.Kit.SourceKind
			}
			baseArgs := []string{rep.KitID, "--project", "--runtime", rt, "--path", projRoot}
			importedDir := filepath.Join(projRoot, ".dtkit", "imported", rep.KitID)
			if rep.KitID == "engineer" && (sourceKind == "" || sourceKind == "bundled") {
				// Bundled engineer kit uses default resolution.
			} else if info, statErr := os.Stat(filepath.Join(importedDir, "package.json")); statErr == nil && info.Mode().IsRegular() {
				baseArgs = append(baseArgs, "--from", importedDir)
			} else {
				outcomes = append(outcomes, UpdateKitOutcome{
					Runtime: rt,
					KitID:   rep.KitID,
					From:    rep.KitVersion,
					Status:  "skipped",
					Detail:  "external source not stored under .dtkit/imported; use 'dtkit update <kit> --from|--source ...' if needed",
				})
				continue
			}

			var planBuf bytes.Buffer
			planDeps := deps
			planDeps.Stdout = &planBuf
			planDeps.Stderr = ioDiscard{}
			planErr := runUpdate(ctx, append(append([]string{}, baseArgs...), "--dry-run", "--json"), planDeps)
			if planErr != nil {
				outcomes = append(outcomes, UpdateKitOutcome{
					Runtime: rt,
					KitID:   rep.KitID,
					From:    rep.KitVersion,
					Status:  "conflict",
					Detail:  planErr.Error(),
				})
				warnings = append(warnings, fmt.Sprintf("%s/%s: %v", rt, rep.KitID, planErr))
				continue
			}
			var planEnv Envelope
			if decErr := json.Unmarshal(planBuf.Bytes(), &planEnv); decErr != nil || !planEnv.Ok || planEnv.Kit == nil {
				outcomes = append(outcomes, UpdateKitOutcome{
					Runtime: rt,
					KitID:   rep.KitID,
					From:    rep.KitVersion,
					Status:  "conflict",
					Detail:  "failed to decode update plan",
				})
				continue
			}

			needsMutation := planEnv.Kit.Version != rep.KitVersion || planEnv.Kit.Digest != rep.KitDigest
			if planEnv.Summary != nil && (planEnv.Summary.Create+planEnv.Summary.Update+planEnv.Summary.Remove) > 0 {
				needsMutation = true
			}
			if !needsMutation && rep.Healthy {
				outcomes = append(outcomes, UpdateKitOutcome{
					Runtime: rt,
					KitID:   rep.KitID,
					From:    rep.KitVersion,
					To:      planEnv.Kit.Version,
					Status:  "up_to_date",
				})
				continue
			}
			if dryRun {
				outcomes = append(outcomes, UpdateKitOutcome{
					Runtime: rt,
					KitID:   rep.KitID,
					From:    rep.KitVersion,
					To:      planEnv.Kit.Version,
					Status:  "would_update",
				})
				continue
			}

			var applyBuf bytes.Buffer
			applyDeps := deps
			applyDeps.Stdout = &applyBuf
			applyDeps.Stderr = ioDiscard{}
			if applyErr := runUpdate(ctx, append(append([]string{}, baseArgs...), "--yes", "--json"), applyDeps); applyErr != nil {
				outcomes = append(outcomes, UpdateKitOutcome{
					Runtime: rt,
					KitID:   rep.KitID,
					From:    rep.KitVersion,
					To:      planEnv.Kit.Version,
					Status:  "conflict",
					Detail:  applyErr.Error(),
				})
				warnings = append(warnings, fmt.Sprintf("%s/%s update skipped: %v", rt, rep.KitID, applyErr))
				continue
			}
			outcomes = append(outcomes, UpdateKitOutcome{
				Runtime: rt,
				KitID:   rep.KitID,
				From:    rep.KitVersion,
				To:      planEnv.Kit.Version,
				Status:  "updated",
			})
		}
	}
	return outcomes, warnings, firstInstalledRT
}

func syncProjectLockfile(ctx context.Context, deps Deps, projRoot, runtimeID string, dryRun bool) (bool, []string) {
	lockPath := filepath.Join(projRoot, dist.LockfileRelPath)
	oldRaw, rErr := readRegularFileBounded(lockPath, dist.MaxLockfileBytes)
	if rErr != nil {
		return false, nil
	}
	if runtimeID == "" {
		return false, nil
	}

	roots := map[string]string{"project": projRoot}
	reps, err := installer.InspectRuntimeAll(runtimeID, "project", roots)
	if err != nil {
		return false, []string{fmt.Sprintf("dtkit.lock refresh skipped: %v", err)}
	}
	lockArgs := []string{"generate", "--path", projRoot, "--runtime", runtimeID, "--json"}
	for _, rep := range reps {
		if !rep.Installed || rep.KitID == "engineer" {
			continue
		}
		importedDir := filepath.Join(projRoot, ".dtkit", "imported", rep.KitID)
		if info, statErr := os.Stat(filepath.Join(importedDir, "package.json")); statErr == nil && info.Mode().IsRegular() {
			lockArgs = append(lockArgs, "--source-ref", fmt.Sprintf("%s=.dtkit/imported/%s", rep.KitID, rep.KitID))
		}
	}

	var buf bytes.Buffer
	lockDeps := deps
	lockDeps.Stdout = &buf
	lockDeps.Stderr = ioDiscard{}

	planArgs := append(append([]string{}, lockArgs...), "--dry-run")
	if lErr := runLock(ctx, planArgs, lockDeps); lErr != nil {
		return false, []string{fmt.Sprintf("dtkit.lock refresh skipped: %v", lErr)}
	}
	var planEnv Envelope
	if dErr := json.Unmarshal(buf.Bytes(), &planEnv); dErr != nil || !planEnv.Ok || planEnv.Lockfile == nil {
		return false, nil
	}
	nextRaw, mErr := dist.MarshalLockfile(*planEnv.Lockfile)
	if mErr != nil {
		return false, nil
	}
	changed := !bytes.Equal(bytes.TrimSpace(oldRaw), bytes.TrimSpace(nextRaw))
	if !changed || dryRun {
		return changed, nil
	}

	buf.Reset()
	applyArgs := append(append([]string{}, lockArgs...), "--yes")
	if lErr := runLock(ctx, applyArgs, lockDeps); lErr != nil {
		return false, []string{fmt.Sprintf("dtkit.lock refresh failed: %v", lErr)}
	}
	return true, nil
}

func printUpdateSummaryText(deps Deps, summary UpdateSummary, dryRun bool) {
	verb := "Updated"
	stateLabel := func(changed bool) string {
		if !changed {
			return "up to date"
		}
		if dryRun {
			return "would update"
		}
		return "updated"
	}
	if dryRun {
		verb = "Preview dtkit update"
	}
	fmt.Fprintf(deps.Stdout, "%s at %s\n", verb, summary.Root)
	fmt.Fprintf(deps.Stdout, "  - Codebase summary (docs/codebase-summary.md): %s\n", stateLabel(summary.CodebaseSummary))
	profileNote := stateLabel(summary.ProjectProfile)
	if len(summary.AddedRoots) > 0 {
		profileNote += fmt.Sprintf(" (+roots: %s)", strings.Join(summary.AddedRoots, ", "))
	}
	fmt.Fprintf(deps.Stdout, "  - Project profile (.dtkit/project.json): %s\n", profileNote)

	if len(summary.Kits) == 0 {
		fmt.Fprintln(deps.Stdout, "  - Installed kits: none (run 'dtkit init' to install engineer)")
	} else {
		updated, upToDate, skipped, conflicts := 0, 0, 0, 0
		for _, k := range summary.Kits {
			switch k.Status {
			case "updated", "would_update":
				updated++
			case "up_to_date":
				upToDate++
			case "skipped":
				skipped++
			case "conflict":
				conflicts++
			}
		}
		kitVerb := "updated"
		if dryRun {
			kitVerb = "would update"
		}
		fmt.Fprintf(deps.Stdout, "  - Installed kits (%d checked): %d %s, %d up to date, %d skipped, %d conflict(s)\n", len(summary.Kits), updated, kitVerb, upToDate, skipped, conflicts)
	}
	if summary.LockfileUpdated {
		fmt.Fprintf(deps.Stdout, "  - Lockfile (dtkit.lock): %s\n", stateLabel(true))
	}
}
