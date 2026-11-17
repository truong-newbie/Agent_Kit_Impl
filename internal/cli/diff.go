package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

func runDiff(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("diff")
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

	var kitSpec, kitFlag, scope, projectPath, runtimeArg, fromPath, sourceURL, sha256Hex, presetRaw, componentsRaw string
	var offline, includeNoop, hasPreset, hasComponents bool

	for i := 0; i < len(args); i++ {
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
		case args[i] == "--json":
			jsonOutput = true
		case args[i] == "--offline":
			offline = true
		case args[i] == "--redact":
			redact = true
		case args[i] == "--include-unchanged":
			includeNoop = true
		case args[i] == "--verbose":
			deps.Verbose = true
		case strings.HasPrefix(args[i], "--preset="):
			hasPreset = true
			presetRaw = strings.TrimPrefix(args[i], "--preset=")
		case strings.HasPrefix(args[i], "--components="):
			hasComponents = true
			componentsRaw = strings.TrimPrefix(args[i], "--components=")
		case args[i] == "--kit" || args[i] == "--runtime" || args[i] == "--path" || args[i] == "--from" || args[i] == "--source" || args[i] == "--sha256" || args[i] == "--preset" || args[i] == "--components":
			flag := args[i]
			i++
			if i >= len(args) {
				return fail(apperr.Usage("missing_flag_value", fmt.Sprintf("%s requires a value", flag)))
			}
			switch flag {
			case "--kit":
				kitFlag = args[i]
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
		case !strings.HasPrefix(args[i], "-"):
			if kitSpec != "" {
				return fail(apperr.Usage("unexpected_argument", fmt.Sprintf("unexpected extra argument %q", args[i])))
			}
			kitSpec = args[i]
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown diff option %q", args[i])))
		}
	}

	if kitSpec != "" && kitFlag != "" {
		return fail(apperr.Usage("invalid_flag_combination", "pass either positional <kit>[@<ver>] or --kit, not both"))
	}
	if kitFlag != "" {
		kitSpec = kitFlag
	}

	preset, requestedComponents, selErr := parseSelectionInputs(presetRaw, hasPreset, componentsRaw, hasComponents)
	if selErr != nil {
		return fail(selErr)
	}

	profile, rtErr := resolveRuntimeProfile(runtimeArg)
	if rtErr != nil {
		return fail(rtErr)
	}
	env.DryRun = true
	env.Runtime = profile.ID
	env.Scope = scope

	if scope == "" {
		return fail(apperr.WithHint(apperr.Usage("missing_scope", "choose --project or --global"), "example: dtkit diff --project"))
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

	statePath, err := resolveRuntimeStatePath(profile, scope, roots)
	if err != nil {
		return fail(err)
	}
	reg, regExists, err := state.LoadRegistryForRuntime(statePath, scope, profile.ID)
	if err != nil {
		return fail(err)
	}
	if regExists && len(reg.Kits) > 0 {
		sole := reg.Kits[reg.KitIDs()[0]]
		if err := checkRuntimeHomeBinding(profile, scope, &sole, roots); err != nil {
			return fail(err)
		}
	}

	var existing *state.Installation
	var peerWhole map[string]string

	if kitSpec == "" {
		if !regExists || len(reg.Kits) == 0 {
			if fromPath == "" && sourceURL == "" {
				installedFlag := false
				env.Installed = &installedFlag
				if redact {
					redactEnvelope(&env, collectHomeCandidates(deps))
				}
				if jsonOutput {
					return writeEnvelope(deps.Stdout, env)
				}
				fmt.Fprintf(deps.Stdout, "No kit installed in %s scope (%s).\n", scope, profile.DisplayName)
				return nil
			}
			// Source passed without positional kit name: let --from/--source determine package ID via empty spec not allowed, require kit name or resolve.
			return fail(apperr.Usage("missing_kit", "specify <kit>[@<version>] when previewing with --from or --source"))
		}
		if len(reg.Kits) > 1 {
			return fail(apperr.Usage("ambiguous_kit", fmt.Sprintf("multiple kits installed (%s); pass <kit> or --kit <id>", strings.Join(reg.KitIDs(), ", "))))
		}
		soleID := reg.KitIDs()[0]
		kitVal := reg.Kits[soleID]
		existing = &kitVal
		kitSpec = soleID
		peerWhole = reg.PeerOwnedWholeFiles(soleID)
	}

	var resolved dist.ResolvedPackage
	sourceResolved := false
	if existing != nil && fromPath == "" && sourceURL == "" && sha256Hex == "" && !offline && existing.Kit.ID != "engineer" {
		// Installed custom kit without external source: inspect drift against state metadata without guessing bytes.
		reports, err := installer.InspectRuntimeAll(profile.ID, scope, roots)
		if err != nil {
			return fail(err)
		}
		diff, explain := diffFromStateInspection(existing, reports, peerWhole)
		installedFlag := true
		env.Installed = &installedFlag
		env.Generation = existing.Generation
		env.Kit = &KitSummary{ID: existing.Kit.ID, Version: existing.Kit.Version, Digest: existing.Kit.Digest, ArchiveSHA256: existing.Kit.ArchiveSHA256}
		env.Selection = existing.Selection
		env.Explain = &explain
		env.Diff = &diff
		env.Warnings = append(env.Warnings, fmt.Sprintf("source package for %s %s is not bundled; pass --from <dir|archive> or --source <url> for line-level patches", existing.Kit.ID, existing.Kit.Version))
		if redact {
			redactEnvelope(&env, collectHomeCandidates(deps))
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		printPlanDiffText(deps.Stdout, env.Explain, env.Diff)
		return nil
	}

	resolved, err = resolveKitSource(ctx, deps, kitSpec, fromPath, sourceURL, sha256Hex, offline, dist.CacheReadOnly)
	if err != nil {
		return fail(err)
	}
	sourceResolved = true
	pkg := resolved.Package
	source := resolved.Source
	if err := applyScopeCapabilitiesAndBinaryWarnings(deps, profile, scope, &pkg, &env); err != nil {
		return fail(err)
	}

	if existing == nil && regExists {
		if exVal, ok := reg.Kits[pkg.ID]; ok {
			existing = &exVal
			installedFlag := true
			env.Installed = &installedFlag
			env.Generation = existing.Generation
		}
		peerWhole = reg.PeerOwnedWholeFiles(pkg.ID)
	} else if existing != nil {
		installedFlag := true
		env.Installed = &installedFlag
		env.Generation = existing.Generation
	} else {
		installedFlag := false
		env.Installed = &installedFlag
	}

	if !hasPreset && !hasComponents && existing != nil && existing.Selection != nil {
		preset = existing.Selection.Preset
		requestedComponents = existing.Selection.RequestedComponents
	}

	selectionSpec, desired, err := renderWithSelection(profile.ID, pkg, source, scope, preset, requestedComponents)
	if err != nil {
		return fail(err)
	}
	env.Selection = selectionSpec
	env.Kit = &KitSummary{
		ID:            pkg.ID,
		Version:       pkg.Version,
		Digest:        resolved.PackageDigest,
		ArchiveSHA256: resolved.ArchiveSHA256,
	}

	plan, err := planner.PlanDesiredWithPeersForRuntime(profile.ID, scope, pkg.ID, roots, existing, peerWhole, desired)
	if err != nil {
		return fail(err)
	}
	env.Actions, env.Summary = summarizeActions(plan.Actions)

	diff, err := planner.ComputePlanDiff(profile.ID, scope, pkg.ID, roots, plan, desired, includeNoop)
	if err != nil {
		return fail(err)
	}
	_ = sourceResolved
	explain := buildExplainView(&resolved, fromPath, sourceURL, existing, selectionSpec, plan, desired)
	env.Explain = &explain
	env.Diff = &diff

	if redact {
		redactEnvelope(&env, collectHomeCandidates(deps))
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	for _, w := range env.Warnings {
		fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
	}
	printPlanDiffText(deps.Stdout, env.Explain, env.Diff)
	return nil
}

func diffFromStateInspection(existing *state.Installation, reports []installer.StatusReport, peerWhole map[string]string) (planner.PlanDiff, ExplainView) {
	explain := ExplainView{
		KitID:            existing.Kit.ID,
		SourceKind:       "installed-state",
		InstalledVersion: existing.Kit.Version,
		InstalledDigest:  existing.Kit.Digest,
		Selection:        existing.Selection,
	}
	for _, a := range existing.Artifacts {
		if a.Strategy == "managed-block" {
			explain.OwnedManagedBlocks++
		} else {
			explain.OwnedWholeFiles++
		}
	}
	_ = peerWhole

	diff := planner.PlanDiff{}
	for _, r := range reports {
		if r.KitID != existing.Kit.ID {
			continue
		}
		for _, a := range r.Artifacts {
			if a.Status == "healthy" {
				continue
			}
			explain.DriftFiles++
			diff.Changes = append(diff.Changes, planner.ArtifactDiff{
				Root:   a.Root,
				Path:   a.Path,
				Owner:  existing.Kit.ID,
				Status: "conflict",
				Reason: a.Status,
			})
		}
	}
	return diff, explain
}

func buildExplainView(resolved *dist.ResolvedPackage, fromPath, sourceURL string, existing *state.Installation, sel *catalog.SelectionSpec, plan planner.InstallPlan, desired []adapter.Artifact) ExplainView {
	explain := ExplainView{
		Selection: sel,
	}
	if resolved != nil {
		explain.KitID = resolved.Package.ID
		explain.SourceKind = resolved.SourceKind
		explain.TargetVersion = resolved.Package.Version
		explain.TargetDigest = resolved.PackageDigest
		switch {
		case sourceURL != "":
			explain.SourceLocation = dist.SanitizeURL(sourceURL)
		case fromPath != "":
			explain.SourceLocation = filepath.Clean(fromPath)
		}
	}
	if existing != nil {
		if explain.KitID == "" {
			explain.KitID = existing.Kit.ID
		}
		explain.InstalledVersion = existing.Kit.Version
		explain.InstalledDigest = existing.Kit.Digest
	}
	for _, a := range desired {
		if a.Strategy == "managed-block" {
			explain.OwnedManagedBlocks++
		} else {
			explain.OwnedWholeFiles++
		}
	}
	for _, act := range plan.Actions {
		if act.Kind != "conflict" {
			continue
		}
		switch {
		case strings.HasPrefix(act.Reason, "artifact_owned_by_"):
			explain.PeerOwnedBlocked++
		case act.Reason == "managed_file_modified", act.Reason == "managed_block_modified", act.Reason == "managed_target_missing":
			explain.DriftFiles++
		}
	}
	return explain
}

func printPlanDiffText(out io.Writer, explain *ExplainView, diff *planner.PlanDiff) {
	if explain != nil {
		fmt.Fprintf(out, "Kit: %s\n", explain.KitID)
		if explain.InstalledVersion != "" || explain.TargetVersion != "" {
			fmt.Fprintf(out, "Version: installed=%s target=%s\n", fallbackDash(explain.InstalledVersion), fallbackDash(explain.TargetVersion))
		}
		if explain.SourceLocation != "" {
			fmt.Fprintf(out, "Source: %s (%s)\n", explain.SourceKind, explain.SourceLocation)
		} else {
			fmt.Fprintf(out, "Source: %s\n", explain.SourceKind)
		}
		if explain.Selection != nil {
			selDesc := strings.Join(explain.Selection.ResolvedComponents, ",")
			if explain.Selection.Preset != "" {
				fmt.Fprintf(out, "Selection: preset=%s (%d components: %s)\n", explain.Selection.Preset, len(explain.Selection.ResolvedComponents), selDesc)
			} else {
				fmt.Fprintf(out, "Selection: custom (%d components: %s)\n", len(explain.Selection.ResolvedComponents), selDesc)
			}
		}
		fmt.Fprintf(out, "Ownership: whole-files=%d managed-blocks=%d peer-blocked=%d drift=%d\n",
			explain.OwnedWholeFiles, explain.OwnedManagedBlocks, explain.PeerOwnedBlocked, explain.DriftFiles)
	}
	if diff == nil || len(diff.Changes) == 0 {
		fmt.Fprintln(out, "No content differences.")
		return
	}
	for _, ch := range diff.Changes {
		fmt.Fprintf(out, "\n[%s] %s:%s (reason=%s, owner=%s)\n", strings.ToUpper(ch.Status), ch.Root, ch.Path, ch.Reason, ch.Owner)
		if ch.Binary {
			fmt.Fprintln(out, "  (binary content omitted)")
			continue
		}
		if ch.Patch != "" {
			fmt.Fprint(out, ch.Patch)
			if !strings.HasSuffix(ch.Patch, "\n") {
				fmt.Fprintln(out)
			}
		}
	}
	if diff.Truncated {
		fmt.Fprintf(out, "\nNote: diff output was capped at %d bytes total.\n", planner.MaxTotalDiffBytes)
	}
}

func fallbackDash(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

func cloneRootsMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
