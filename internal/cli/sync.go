package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/buildinfo"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/dist"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/installer"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

type preparedSyncKit struct {
	entry     dist.LockedKit
	resolved  dist.ResolvedPackage
	selection *catalog.SelectionSpec
	artifacts []adapter.Artifact
	action    string // "install" | "update" | "noop"
}

func runSync(ctx context.Context, args []string, deps Deps) error {
	env := newEnvelope("sync")
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

	var projectPath, runtimeArg, lockPath, trustPath string
	var offline, requireSig, dryRun, showDiff, yes bool
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
		case "--path", "--runtime", "--lock", "--trust":
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
			case "--lock":
				lockPath = args[i]
			case "--trust":
				trustPath = args[i]
			}
		default:
			return fail(apperr.Usage("unknown_option", fmt.Sprintf("unknown sync option %q", args[i])))
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

	lf, policy, err := loadLockAndTrust(projRoot, lockPath, trustPath)
	if err != nil {
		return fail(err)
	}
	env.Lockfile = &lf

	pending, lockStatus, err := installer.InspectTransactionsForRuntime(profile.ID, "project", roots)
	if err != nil {
		return fail(err)
	}
	if len(pending) > 0 || lockStatus.Present {
		return fail(apperr.WithHint(apperr.Conflict("recovery_required", "unfinished transaction or lock exists"), fmt.Sprintf("run dtkit recover --project --runtime %s --dry-run", profile.ID)))
	}

	statePath, err := resolveRuntimeStatePath(profile, "project", roots)
	if err != nil {
		return fail(err)
	}
	reg, regExists, err := state.LoadRegistryForRuntime(statePath, "project", profile.ID)
	if err != nil {
		return fail(err)
	}

	cache, err := dist.OpenCache(deps.Env)
	if err != nil {
		return fail(err)
	}

	// Verify ALL entries (compatibility, digests, provenance, and planner conflicts) BEFORE mutating any workspace file.
	prepared := make([]preparedSyncKit, 0, len(lf.Kits))
	peerWhole := map[string]string{}
	if regExists {
		for k, v := range reg.PeerOwnedWholeFiles("") {
			peerWhole[k] = v
		}
	}

	var combinedActions []planner.Action
	var combinedDiff planner.PlanDiff
	for _, entry := range lf.Kits {
		if err := entry.CheckRuntimeCompatibility(profile.ID); err != nil {
			return fail(err)
		}
		resolved, rErr := dist.ResolveLockedKit(ctx, deps.HTTPClient, cache, projRoot, entry, buildinfo.Version, offline, dist.CacheReadOnly)
		if rErr != nil {
			return fail(rErr)
		}
		if pErr := dist.VerifyProvenance(entry.ID, entry.Version, entry.LockTargetDigest(), entry.Provenance, policy, requireSig, deps.Now()); pErr != nil {
			return fail(pErr)
		}
		pkg := resolved.Package
		if err := applyScopeCapabilitiesAndBinaryWarnings(deps, profile, "project", &pkg, &env); err != nil {
			return fail(err)
		}
		var preset string
		var reqComp []string
		if entry.Selection != nil {
			preset = entry.Selection.Preset
			reqComp = entry.Selection.RequestedComponents
		}
		selSpec, desired, err := renderWithSelection(profile.ID, pkg, resolved.Source, "project", preset, reqComp)
		if err != nil {
			return fail(err)
		}

		var existingPtr *state.Installation
		if regExists {
			if exVal, ok := reg.Kits[entry.ID]; ok {
				exCopy := exVal
				existingPtr = &exCopy
			}
		}

		// Peer ownership excludes kits already owned by this kit ID.
		kitPeers := make(map[string]string, len(peerWhole))
		for k, owner := range peerWhole {
			if owner != entry.ID {
				kitPeers[k] = owner
			}
		}

		plan, err := planner.PlanDesiredWithPeersForRuntime(profile.ID, "project", entry.ID, roots, existingPtr, kitPeers, desired)
		if err != nil {
			return fail(err)
		}
		combinedActions = append(combinedActions, plan.Actions...)
		if showDiff {
			d, dErr := planner.ComputePlanDiff(profile.ID, "project", entry.ID, roots, plan, desired, false)
			if dErr != nil {
				return fail(dErr)
			}
			combinedDiff.Changes = append(combinedDiff.Changes, d.Changes...)
			combinedDiff.TotalBytes += d.TotalBytes
			if d.Truncated {
				combinedDiff.Truncated = true
			}
		}
		if plan.Conflicts != 0 {
			return fail(apperr.Conflict("sync_conflicts", fmt.Sprintf("sync for %s found %d conflict(s)", entry.ID, plan.Conflicts)))
		}

		actionKind := "install"
		if existingPtr != nil {
			hasMutation := false
			for _, a := range plan.Actions {
				if a.Kind != "noop" {
					hasMutation = true
					break
				}
			}
			if !hasMutation && existingPtr.Kit.Version == pkg.Version && existingPtr.Kit.Digest == resolved.PackageDigest && existingPtr.AdapterRevision == profile.Revision && catalog.SelectionsEqual(existingPtr.Selection, selSpec) {
				actionKind = "noop"
			} else {
				actionKind = "update"
			}
		}
		for _, a := range desired {
			if a.Strategy == "whole-file" {
				peerWhole[strings.ToLower("project\x00"+a.Path)] = entry.ID
			}
		}
		prepared = append(prepared, preparedSyncKit{
			entry:     entry,
			resolved:  resolved,
			selection: selSpec,
			artifacts: desired,
			action:    actionKind,
		})
	}

	env.Actions, env.Summary = summarizeActions(combinedActions)
	if showDiff {
		env.Diff = &combinedDiff
	}

	if dryRun {
		for _, p := range prepared {
			res := SyncKitResult{
				KitID:         p.entry.ID,
				Version:       p.entry.Version,
				PackageDigest: p.resolved.PackageDigest,
				ArchiveSHA256: p.resolved.ArchiveSHA256,
				Action:        p.action,
			}
			if p.entry.Provenance != nil {
				res.ProvenanceKey = p.entry.Provenance.KeyID
			}
			env.SyncResults = append(env.SyncResults, res)
		}
		if redact {
			env.Roots = cloneRootsMap(roots)
			redactEnvelope(&env, collectHomeCandidates(deps))
		}
		if jsonOutput {
			return writeEnvelope(deps.Stdout, env)
		}
		fmt.Fprintf(deps.Stdout, "Read-only sync preview (%d locked kit(s), %s):\n", len(prepared), profile.DisplayName)
		for _, r := range env.SyncResults {
			fmt.Fprintf(deps.Stdout, "  %s\t%s\t%s\n", r.Action, r.KitID, r.Version)
		}
		if showDiff {
			printPlanDiffText(deps.Stdout, nil, env.Diff)
		}
		return nil
	}

	needsApply := false
	for _, p := range prepared {
		if p.action != "noop" {
			needsApply = true
			break
		}
	}
	if needsApply {
		if jsonOutput && !yes {
			return fail(apperr.Usage("confirmation_required", "--json sync mutation requires --yes"))
		}
		if !yes && !deps.IsInteractive() {
			return fail(apperr.Usage("confirmation_required", "non-interactive sync requires --yes; run --dry-run first"))
		}
		if !yes {
			confirmed, err := confirmPrompt(ctx, deps, fmt.Sprintf("Sync %d locked kit(s) into project?", len(prepared)))
			if err != nil {
				return fail(err)
			}
			if !confirmed {
				return fail(apperr.Cancelled("sync cancelled by user"))
			}
		}
	}

	if ctx.Err() != nil {
		return fail(apperr.Cancelled("sync cancelled"))
	}

	for _, p := range prepared {
		res := SyncKitResult{
			KitID:         p.entry.ID,
			Version:       p.entry.Version,
			PackageDigest: p.resolved.PackageDigest,
			ArchiveSHA256: p.resolved.ArchiveSHA256,
			Action:        p.action,
		}
		if p.entry.Provenance != nil {
			res.ProvenanceKey = p.entry.Provenance.KeyID
		}
		switch p.action {
		case "install":
			if err := persistResolvedCache(deps.Env, p.resolved); err != nil {
				return fail(err)
			}
			installed, err := installer.FreshInstallWithOptions("project", roots, p.resolved.Package, p.resolved.Source, p.artifacts, deps.Now(), installer.InstallOptions{
				Runtime:       profile.ID,
				SourceKind:    p.resolved.SourceKind,
				ArchiveSHA256: p.resolved.ArchiveSHA256,
				Selection:     p.selection,
			})
			if err != nil {
				return fail(err)
			}
			res.Generation = installed.Generation
		case "update":
			if err := persistResolvedCache(deps.Env, p.resolved); err != nil {
				return fail(err)
			}
			upd, err := installer.Update("project", roots, p.resolved.Package, p.resolved.Source, p.artifacts, deps.Now(), installer.UpdateOptions{
				Runtime:       profile.ID,
				SourceKind:    p.resolved.SourceKind,
				ArchiveSHA256: p.resolved.ArchiveSHA256,
				Selection:     p.selection,
			})
			if err != nil {
				return fail(err)
			}
			res.Generation = upd.Installation.Generation
		case "noop":
			if regExists {
				if ex, ok := reg.Kits[p.entry.ID]; ok {
					res.Generation = ex.Generation
				}
			}
		}
		env.SyncResults = append(env.SyncResults, res)
	}

	if redact {
		env.Roots = cloneRootsMap(roots)
		redactEnvelope(&env, collectHomeCandidates(deps))
	}
	if jsonOutput {
		return writeEnvelope(deps.Stdout, env)
	}
	for _, w := range env.Warnings {
		fmt.Fprintf(deps.Stderr, "warning: %s\n", w)
	}
	fmt.Fprintf(deps.Stdout, "Synced %d locked kit(s) for %s.\n", len(env.SyncResults), profile.DisplayName)
	return nil
}
