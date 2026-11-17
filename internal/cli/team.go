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
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func runTeam(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("team")
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

	if len(args) == 0 {
		return fail(apperr.Usage("missing_subcommand", "usage: dtkit team <export|import|template> [options]"))
	}
	sub := args[0]
	env.Command = "team " + sub

	switch sub {
	case "export":
		return runTeamExport(ctx, args[1:], deps, env, fail)
	case "import":
		return runTeamImport(ctx, args[1:], deps, env, fail)
	case "template":
		return runTeamTemplate(ctx, args[1:], deps, env, fail)
	default:
		return fail(apperr.Usage("unknown_subcommand", fmt.Sprintf("unknown team subcommand %q (expected export, import, template)", sub)))
	}
}

func runTeamExport(ctx context.Context, args []string, deps Deps, env Envelope, fail func(error) error) error {
	var projectPath, runtimeArg, outPath, name, runtimesRaw, hooksRaw string
	var dryRun, yes, jsonOutput, redact bool
	includeTrust := true
	includeProject := true
	sourceRefs := map[string]string{}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			dryRun = true
		case "--yes":
			yes = true
		case "--json":
			jsonOutput = true
		case "--redact":
			redact = true
		case "--no-trust":
			includeTrust = false
		case "--no-project":
			includeProject = false
		case "--verbose":
			deps.Verbose = true
		case "--path", "--runtime", "--out", "--name", "--runtimes", "--hooks", "--source-ref":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--path":
				projectPath = args[i]
			case "--runtime":
				runtimeArg = args[i]
			case "--out":
				outPath = args[i]
			case "--name":
				name = args[i]
			case "--runtimes":
				runtimesRaw = args[i]
			case "--hooks":
				hooksRaw = args[i]
			case "--source-ref":
				kitID, ref, ok := strings.Cut(args[i], "=")
				if !ok || strings.TrimSpace(kitID) == "" || strings.TrimSpace(ref) == "" {
					return fail(apperr.Usage("invalid_source_ref", "--source-ref must use <kit-id>=<relative-path-or-url>"))
				}
				sourceRefs[strings.TrimSpace(kitID)] = strings.TrimSpace(ref)
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown team export option %q", args[i])))
		}
	}

	profile, err := resolveRuntimeProfile(runtimeArg)
	if err != nil {
		return fail(err)
	}
	env.DryRun = dryRun
	env.Runtime = profile.ID
	env.Scope = "project"

	roots, rootWarnings, err := resolveScopeRoots(ctx, deps, profile, "project", projectPath)
	if err != nil {
		return fail(err)
	}
	env.Warnings = append(env.Warnings, rootWarnings...)
	env.Roots = roots
	projRoot := roots["project"]

	// Prefer committed dtkit.lock; fallback read-only to installed kits in (runtime, project).
	var lf dist.Lockfile
	lockFilePath := filepath.Join(projRoot, dist.LockfileRelPath)
	if rawLF, rErr := readRegularFileBounded(lockFilePath, dist.MaxLockfileBytes); rErr == nil {
		lf, err = dist.ParseAndValidateLockfile(rawLF)
		if err != nil {
			return fail(err)
		}
	} else if os.IsNotExist(rErr) {
		statePath, spErr := resolveRuntimeStatePath(profile, "project", roots)
		if spErr != nil {
			return fail(spErr)
		}
		reg, exists, stErr := state.LoadRegistryForRuntime(statePath, "project", profile.ID)
		if stErr != nil {
			return fail(stErr)
		}
		if !exists || len(reg.Kits) == 0 {
			return fail(apperr.Conflict("nothing_to_export", "neither dtkit.lock nor installed project kits found"))
		}
		lf.SchemaVersion = 1
		for _, kitID := range reg.KitIDs() {
			inst := reg.Kits[kitID]
			src, archSHA, targets, sErr := resolveLockSourceForInstalled(projRoot, inst, sourceRefs[kitID])
			if sErr != nil {
				return fail(sErr)
			}
			if len(targets) == 0 {
				targets = []string{profile.ID}
			}
			lf.Kits = append(lf.Kits, dist.LockedKit{
				ID:               inst.Kit.ID,
				Version:          inst.Kit.Version,
				PackageDigest:    inst.Kit.Digest,
				ArchiveSHA256:    archSHA,
				Source:           src,
				RuntimeTargets:   targets,
				AdapterRevisions: map[string]string{profile.ID: profile.Revision},
				Selection:        inst.Selection,
			})
		}
	} else {
		return fail(rErr)
	}

	var policyPtr *dist.TrustPolicy
	if includeTrust {
		trustPath := filepath.Join(projRoot, filepath.FromSlash(dist.TrustPolicyRelPath))
		if rawTrust, tErr := readRegularFileBounded(trustPath, dist.MaxTrustPolicyBytes); tErr == nil {
			pol, pErr := dist.ParseAndValidateTrustPolicy(rawTrust)
			if pErr != nil {
				return fail(pErr)
			}
			policyPtr = &pol
		} else if !os.IsNotExist(tErr) {
			return fail(tErr)
		}
	}

	var projProfPtr *project.Profile
	if includeProject {
		projJSONPath := filepath.Join(projRoot, filepath.FromSlash(project.ProfileRelPath))
		if rawProj, pErr := readRegularFileBounded(projJSONPath, project.MaxProfileBytes); pErr == nil {
			prof, vErr := project.ParseAndValidate(rawProj)
			if vErr != nil {
				return fail(vErr)
			}
			projProfPtr = &prof
		} else if !os.IsNotExist(pErr) {
			return fail(pErr)
		}
	}

	runtimes := []string{profile.ID}
	if runtimesRaw != "" {
		runtimes = splitTrimComma(runtimesRaw)
	} else if len(lf.Kits) > 0 && len(lf.Kits[0].RuntimeTargets) > 0 {
		runtimes = append([]string(nil), lf.Kits[0].RuntimeTargets...)
	}
	var hookRecs []string
	if hooksRaw != "" {
		hookRecs = splitTrimComma(hooksRaw)
	}

	bundle := dist.TeamBundle{
		SchemaVersion:       1,
		Name:                name,
		DefaultRuntime:      profile.ID,
		Runtimes:            runtimes,
		Lockfile:            lf,
		TrustPolicy:         policyPtr,
		ProjectProfile:      projProfPtr,
		HookRecommendations: hookRecs,
	}
	bundleBytes, err := dist.MarshalTeamBundle(bundle)
	if err != nil {
		return fail(err)
	}

	// Strict privacy verification: bundle must never carry home or workspace paths.
	homes := collectHomeCandidates(deps)
	for _, forbidden := range append(homes, projRoot, filepath.ToSlash(projRoot)) {
		if forbidden != "" && strings.Contains(string(bundleBytes), forbidden) {
			return fail(apperr.Conflict("non_portable_export", "exported bundle would contain machine-local paths"))
		}
	}

	parsedBundle, err := dist.ParseAndValidateTeamBundle(bundleBytes)
	if err != nil {
		return fail(err)
	}
	env.TeamBundle = &parsedBundle

	targetPath := filepath.Join(projRoot, filepath.FromSlash(dist.TeamBundleRelPath))
	if outPath != "" {
		targetPath, err = filepath.Abs(outPath)
		if err != nil {
			return fail(err)
		}
	}

	if dryRun {
		if redact {
			env.Roots = cloneRootsMap(roots)
			redactEnvelope(&env, homes)
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Read-only preview: team onboarding bundle (%d locked kit(s), runtimes=%s) -> %s\n", len(parsedBundle.Lockfile.Kits), strings.Join(parsedBundle.Runtimes, ","), targetPath)
		return nil
	}
	if jsonOutput && !yes {
		return fail(apperr.Usage("confirmation_required", "--json team export requires --yes"))
	}
	if !yes && !deps.IsInteractive() {
		return fail(apperr.Usage("confirmation_required", "non-interactive team export requires --yes; run --dry-run first"))
	}
	if !yes {
		confirmed, err := confirmPrompt(ctx, deps, fmt.Sprintf("Write portable team bundle to %s?", targetPath))
		if err != nil {
			return fail(err)
		}
		if !confirmed {
			return fail(apperr.Cancelled("team export cancelled by user"))
		}
	}

	if err := atomicWriteFile(targetPath, bundleBytes, 0644); err != nil {
		return fail(err)
	}
	if redact {
		env.Roots = cloneRootsMap(roots)
		redactEnvelope(&env, homes)
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Exported portable team bundle to %s.\n", targetPath)
	return nil
}

func runTeamImport(ctx context.Context, args []string, deps Deps, env Envelope, fail func(error) error) error {
	var bundlePath, projectPath, runtimeArg string
	var offline, requireSig, dryRun, showDiff, yes, jsonOutput, redact bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--offline":
			offline = true
		case "--require-signature":
			requireSig = true
		case "--dry-run":
			dryRun = true
		case "--diff":
			showDiff = true
		case "--yes":
			yes = true
		case "--json":
			jsonOutput = true
		case "--redact":
			redact = true
		case "--verbose":
			deps.Verbose = true
		case "--path", "--runtime":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			if flag == "--path" {
				projectPath = args[i]
			} else {
				runtimeArg = args[i]
			}
		default:
			if strings.HasPrefix(args[i], "-") || bundlePath != "" {
				return fail(apperr.Usage("unknown_option", fmt.Sprintf("unexpected argument %q", args[i])))
			}
			bundlePath = args[i]
		}
	}
	if bundlePath == "" {
		return fail(apperr.Usage("missing_bundle", "usage: dtkit team import <bundle-file> [--path <dir>] [--runtime <...>] [--offline] [--require-signature] [--dry-run] [--diff] [--yes] [--json]"))
	}

	rawBundle, err := readRegularFileBounded(bundlePath, dist.MaxTeamBundleBytes)
	if err != nil {
		return fail(err)
	}
	bundle, err := dist.ParseAndValidateTeamBundle(rawBundle)
	if err != nil {
		return fail(err)
	}
	env.TeamBundle = &bundle

	if runtimeArg == "" && bundle.DefaultRuntime != "" {
		runtimeArg = bundle.DefaultRuntime
	}
	profile, err := resolveRuntimeProfile(runtimeArg)
	if err != nil {
		return fail(err)
	}
	env.DryRun = dryRun
	env.Runtime = profile.ID
	env.Scope = "project"

	roots, rootWarnings, err := resolveScopeRoots(ctx, deps, profile, "project", projectPath)
	if err != nil {
		return fail(err)
	}
	env.Warnings = append(env.Warnings, rootWarnings...)
	env.Roots = roots
	projRoot := roots["project"]

	// Check runtime inclusion declared by the team bundle.
	rtAllowed := false
	for _, rt := range bundle.Runtimes {
		if rt == profile.ID {
			rtAllowed = true
			break
		}
	}
	if !rtAllowed {
		return fail(apperr.Conflict("runtime_not_in_team_bundle", fmt.Sprintf("runtime %q is not listed in team bundle runtimes (%s)", profile.ID, strings.Join(bundle.Runtimes, ", "))))
	}

	// Verify non-clobber project profile safety BEFORE writing any file.
	var projectBytes []byte
	projTarget := filepath.Join(projRoot, filepath.FromSlash(project.ProfileRelPath))
	if bundle.ProjectProfile != nil {
		b, mErr := json.MarshalIndent(bundle.ProjectProfile, "", "  ")
		if mErr != nil {
			return fail(mErr)
		}
		projectBytes = append(b, '\n')
		if existingProj, rErr := readRegularFileBounded(projTarget, project.MaxProfileBytes); rErr == nil {
			if !bytes.Equal(bytes.TrimSpace(existingProj), bytes.TrimSpace(projectBytes)) {
				return fail(apperr.WithPath(apperr.Conflict("project_profile_exists", "existing .dtkit/project.json differs from imported team bundle"), projTarget))
			}
			projectBytes = nil // already identical
		} else if !os.IsNotExist(rErr) {
			return fail(rErr)
		}
	}

	// Verify all locked kits & provenance BEFORE mutating any file (zero project command execution!).
	prepared, reg, regExists, err := prepareSyncPlan(ctx, deps, profile, roots, bundle.Lockfile, bundle.TrustPolicy, offline, requireSig, showDiff, &env)
	if err != nil {
		return fail(err)
	}

	summary := TeamImportSummary{
		BundleName:          bundle.Name,
		HookRecommendations: append([]string(nil), bundle.HookRecommendations...),
	}
	if bundle.ProjectProfile != nil {
		summary.ProjectCheckTrust = "untrusted (explicit interactive confirmation required on first dtkit check)"
		env.Warnings = append(env.Warnings, "imported .dtkit/project.json checks remain untrusted until explicitly confirmed with 'dtkit check'")
	}

	if dryRun {
		env.SyncResults = buildSyncPreviewResults(prepared)
		env.TeamImport = &summary
		if redact {
			env.Roots = cloneRootsMap(roots)
			redactEnvelope(&env, collectHomeCandidates(deps))
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Read-only team import preview (%d kit(s), %s):\n", len(prepared), profile.DisplayName)
		for _, r := range env.SyncResults {
			fmt.Fprintf(deps.Stdout, "  %s\t%s\t%s\n", r.Action, r.KitID, r.Version)
		}
		if len(summary.HookRecommendations) > 0 {
			fmt.Fprintf(deps.Stdout, "Recommended opt-in hooks (not auto-enabled): %s\n", strings.Join(summary.HookRecommendations, ", "))
		}
		return nil
	}

	if jsonOutput && !yes {
		return fail(apperr.Usage("confirmation_required", "--json team import requires --yes"))
	}
	if !yes && !deps.IsInteractive() {
		return fail(apperr.Usage("confirmation_required", "non-interactive team import requires --yes; run --dry-run first"))
	}
	if !yes {
		confirmed, err := confirmPrompt(ctx, deps, fmt.Sprintf("Import team config and sync %d kit(s)?", len(prepared)))
		if err != nil {
			return fail(err)
		}
		if !confirmed {
			return fail(apperr.Cancelled("team import cancelled by user"))
		}
	}

	lockBytes, err := dist.MarshalLockfile(bundle.Lockfile)
	if err != nil {
		return fail(err)
	}
	lockTarget := filepath.Join(projRoot, dist.LockfileRelPath)
	if err := atomicWriteFile(lockTarget, lockBytes, 0644); err != nil {
		return fail(err)
	}
	summary.WrittenFiles = append(summary.WrittenFiles, dist.LockfileRelPath)

	if bundle.TrustPolicy != nil {
		trustBytes, err := dist.MarshalTrustPolicy(*bundle.TrustPolicy)
		if err != nil {
			return fail(err)
		}
		if err := atomicWriteFile(filepath.Join(projRoot, filepath.FromSlash(dist.TrustPolicyRelPath)), trustBytes, 0644); err != nil {
			return fail(err)
		}
		summary.WrittenFiles = append(summary.WrittenFiles, dist.TrustPolicyRelPath)
	}
	if len(projectBytes) > 0 {
		if err := atomicWriteFile(projTarget, projectBytes, 0644); err != nil {
			return fail(err)
		}
		summary.WrittenFiles = append(summary.WrittenFiles, project.ProfileRelPath)
	}

	results, err := applySyncPlan(deps, profile, roots, prepared, reg, regExists)
	if err != nil {
		return fail(err)
	}
	env.SyncResults = results
	env.TeamImport = &summary

	if redact {
		env.Roots = cloneRootsMap(roots)
		redactEnvelope(&env, collectHomeCandidates(deps))
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Imported team bundle (%s): synced %d kit(s).\n", profile.DisplayName, len(results))
	return nil
}

func runTeamTemplate(ctx context.Context, args []string, deps Deps, env Envelope, fail func(error) error) error {
	var projectPath, runtimeArg, preset string
	var dryRun, yes, jsonOutput, redact bool
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dry-run":
			dryRun = true
		case "--yes":
			yes = true
		case "--json":
			jsonOutput = true
		case "--redact":
			redact = true
		case "--verbose":
			deps.Verbose = true
		case "--path", "--runtime", "--preset":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--path":
				projectPath = args[i]
			case "--runtime":
				runtimeArg = args[i]
			case "--preset":
				preset = args[i]
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown team template option %q", args[i])))
		}
	}

	profile, err := resolveRuntimeProfile(runtimeArg)
	if err != nil {
		return fail(err)
	}
	env.DryRun = dryRun
	env.Runtime = profile.ID
	env.Scope = "project"

	projRes, err := paths.ResolveProject(ctx, deps.Env, projectPath)
	if err != nil {
		return fail(err)
	}
	env.Warnings = append(env.Warnings, projRes.Warnings...)
	env.Roots = map[string]string{"project": projRes.Root}
	projRoot := projRes.Root

	engPkg, engFS, err := catalog.Engineer()
	if err != nil {
		return fail(err)
	}
	engDig, err := catalog.Digest(engFS)
	if err != nil {
		return fail(err)
	}
	selSpec, err := catalog.ResolveComponentSelection(engPkg, engFS, preset, nil)
	if err != nil {
		return fail(err)
	}

	lf := dist.Lockfile{
		SchemaVersion: 1,
		Kits: []dist.LockedKit{{
			ID:               engPkg.ID,
			Version:          engPkg.Version,
			PackageDigest:    engDig,
			Source:           dist.LockSource{Kind: "bundled"},
			RuntimeTargets:   engPkg.RuntimeTargets,
			AdapterRevisions: map[string]string{profile.ID: profile.Revision},
			Selection:        selSpec,
		}},
	}
	lockBytes, err := dist.MarshalLockfile(lf)
	if err != nil {
		return fail(err)
	}

	inspected, err := project.Inspect(ctx, deps.Env, projRoot)
	if err != nil {
		return fail(err)
	}
	profBytes, err := json.MarshalIndent(inspected.Profile, "", "  ")
	if err != nil {
		return fail(err)
	}
	profBytes = append(profBytes, '\n')

	bundle := dist.TeamBundle{
		SchemaVersion:  1,
		Name:           inspected.Profile.Name,
		DefaultRuntime: profile.ID,
		Runtimes:       adapter.SupportedRuntimes(),
		Lockfile:       lf,
		ProjectProfile: &inspected.Profile,
	}
	guideText := dist.OnboardingGuideMarkdown(bundle)

	targets := []struct {
		rel  string
		data []byte
	}{
		{dist.LockfileRelPath, lockBytes},
		{project.ProfileRelPath, profBytes},
		{dist.OnboardingDocRelPath, []byte(guideText)},
	}

	var res TeamTemplateResult
	for _, tItem := range targets {
		abs := filepath.Join(projRoot, filepath.FromSlash(tItem.rel))
		if _, sErr := os.Lstat(abs); sErr == nil {
			res.SkippedFiles = append(res.SkippedFiles, tItem.rel)
		} else if os.IsNotExist(sErr) {
			res.WrittenFiles = append(res.WrittenFiles, tItem.rel)
		} else {
			return fail(sErr)
		}
	}
	env.TeamTemplate = &res

	if dryRun {
		if redact {
			env.Roots = cloneRootsMap(env.Roots)
			redactEnvelope(&env, collectHomeCandidates(deps))
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Read-only team template preview: create=%v skip=%v\n", res.WrittenFiles, res.SkippedFiles)
		return nil
	}
	if jsonOutput && !yes {
		return fail(apperr.Usage("confirmation_required", "--json team template requires --yes"))
	}
	if !yes && !deps.IsInteractive() {
		return fail(apperr.Usage("confirmation_required", "non-interactive team template requires --yes; run --dry-run first"))
	}
	if !yes {
		confirmed, err := confirmPrompt(ctx, deps, fmt.Sprintf("Scaffold onboarding template (%d files)?", len(res.WrittenFiles)))
		if err != nil {
			return fail(err)
		}
		if !confirmed {
			return fail(apperr.Cancelled("team template cancelled by user"))
		}
	}

	writeMap := map[string][]byte{
		dist.LockfileRelPath:      lockBytes,
		project.ProfileRelPath:    profBytes,
		dist.OnboardingDocRelPath: []byte(guideText),
	}
	for _, rel := range res.WrittenFiles {
		abs := filepath.Join(projRoot, filepath.FromSlash(rel))
		if err := atomicWriteFile(abs, writeMap[rel], 0644); err != nil {
			return fail(err)
		}
	}

	if redact {
		env.Roots = cloneRootsMap(env.Roots)
		redactEnvelope(&env, collectHomeCandidates(deps))
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	fmt.Fprintf(deps.Stdout, "Created repo onboarding template (%v), skipped existing (%v).\n", res.WrittenFiles, res.SkippedFiles)
	return nil
}

func splitTrimComma(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
