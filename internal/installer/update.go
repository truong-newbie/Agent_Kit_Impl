package installer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

type UpdateOptions struct {
	Runtime         string // optional; defaults to codex
	SourceKind      string
	ArchiveSHA256   string
	Selection       *catalog.SelectionSpec
	AdapterRevision string // optional override for adapter migration testing; defaults to runtime profile revision
	Hooks           *TxHooks
}

type UpdateResult struct {
	Updated          bool                `json:"updated"`
	Noop             bool                `json:"noop"`
	AdapterMigration bool                `json:"adapterMigration,omitempty"`
	TransactionID    string              `json:"transactionId,omitempty"`
	Installation     state.Installation  `json:"installation"`
	Plan             planner.InstallPlan `json:"plan"`
}

// PlanUpdate checks Codex version/digest/adapter rules and reconciles state, disk, and desired artifacts.
func PlanUpdate(scope string, roots map[string]string, pkg catalog.Package, pkgDigest string, desired []adapter.Artifact, adapterRev string) (planner.InstallPlan, *state.Installation, bool, error) {
	return PlanUpdateForRuntime(adapter.DefaultRuntime, scope, roots, pkg, pkgDigest, desired, adapterRev)
}

// PlanUpdateForRuntime checks version/digest/adapter rules for runtimeName and reconciles state, disk, and desired artifacts.
func PlanUpdateForRuntime(runtimeName, scope string, roots map[string]string, pkg catalog.Package, pkgDigest string, desired []adapter.Artifact, adapterRev string) (planner.InstallPlan, *state.Installation, bool, error) {
	plan, _, existing, adapterMigration, err := planUpdateWithRegistry(runtimeName, scope, roots, pkg, pkgDigest, desired, adapterRev)
	return plan, existing, adapterMigration, err
}

func planUpdateWithRegistry(runtimeName, scope string, roots map[string]string, pkg catalog.Package, pkgDigest string, desired []adapter.Artifact, adapterRev string) (planner.InstallPlan, *state.Registry, *state.Installation, bool, error) {
	profile, pErr := runtimeProfile(runtimeName)
	if pErr != nil {
		return planner.InstallPlan{}, nil, nil, false, pErr
	}
	if adapterRev == "" {
		adapterRev = profile.Revision
	}
	if !profile.SupportsScope(scope) {
		return planner.InstallPlan{}, nil, nil, false, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	pending, lockStatus, err := InspectTransactionsForRuntime(profile.ID, scope, roots)
	if err != nil {
		return planner.InstallPlan{}, nil, nil, false, err
	}
	if len(pending) > 0 || lockStatus.Present {
		return planner.InstallPlan{}, nil, nil, false, apperr.WithHint(apperr.Conflict("recovery_required", "unfinished transaction or lock exists"), fmt.Sprintf("run dtkit recover --%s --runtime %s --dry-run", scope, profile.ID))
	}

	statePath, err := runtimeStateTarget(profile.ID, scope, roots)
	if err != nil {
		return planner.InstallPlan{}, nil, nil, false, err
	}
	reg, exists, err := state.LoadRegistryForRuntime(statePath, scope, profile.ID)
	if err != nil {
		return planner.InstallPlan{}, nil, nil, false, err
	}
	if !exists {
		return planner.InstallPlan{}, nil, nil, false, apperr.WithHint(apperr.Conflict("not_installed", fmt.Sprintf("kit %q is not installed in %s scope (%s)", pkg.ID, scope, profile.ID)), fmt.Sprintf("run dtkit install %s --%s --runtime %s", pkg.ID, scope, profile.ID))
	}
	if err := checkGlobalHomeBinding(profile, scope, reg.GlobalHome(), roots); err != nil {
		return planner.InstallPlan{}, reg, nil, false, err
	}
	existingVal, ok := reg.Kits[pkg.ID]
	if !ok {
		if len(reg.Kits) == 1 {
			soleID := reg.KitIDs()[0]
			sole := reg.Kits[soleID]
			return planner.InstallPlan{}, reg, &sole, false, apperr.Conflict("kit_id_mismatch", fmt.Sprintf("installed kit %q does not match requested %q", soleID, pkg.ID))
		}
		return planner.InstallPlan{}, reg, nil, false, apperr.WithHint(apperr.Conflict("not_installed", fmt.Sprintf("kit %q is not installed in %s scope (%s)", pkg.ID, scope, profile.ID)), fmt.Sprintf("run dtkit install %s --%s --runtime %s", pkg.ID, scope, profile.ID))
	}
	existing := &existingVal

	cmp, err := compareSemver(pkg.Version, existing.Kit.Version)
	if err != nil {
		return planner.InstallPlan{}, reg, existing, false, err
	}
	if cmp < 0 {
		return planner.InstallPlan{}, reg, existing, false, apperr.Conflict("downgrade_not_supported", fmt.Sprintf("cannot downgrade %s from %s to %s (downgrades are not supported)", pkg.ID, existing.Kit.Version, pkg.Version))
	}
	adapterMigration := false
	if cmp == 0 {
		if pkgDigest != existing.Kit.Digest && !hasProjectOverridesDir(scope, roots, pkg.ID) {
			return planner.InstallPlan{}, reg, existing, false, apperr.WithHint(
				apperr.Conflict("immutable_version", fmt.Sprintf("package version %s has a different digest than installed state; bump package version to update", pkg.Version)),
				"bump kit version in package.json; to replace an unreleased dev snapshot at the same version, run 'dtkit remove' then 'dtkit install'",
			)
		}
		if adapterRev != existing.AdapterRevision {
			adapterMigration = true
		}
	}

	peerWhole := reg.PeerOwnedWholeFiles(pkg.ID)
	plan, err := planner.PlanDesiredWithPeersForRuntime(profile.ID, scope, pkg.ID, roots, existing, peerWhole, desired)
	if err != nil {
		return planner.InstallPlan{}, reg, existing, adapterMigration, err
	}
	return plan, reg, existing, adapterMigration, nil
}

// Update upgrades or migrates an existing installation inside a P1 transaction.
func Update(scope string, roots map[string]string, pkg catalog.Package, source fs.FS, desired []adapter.Artifact, now time.Time, opts UpdateOptions) (UpdateResult, error) {
	var res UpdateResult
	if now.IsZero() {
		now = time.Now()
	}
	profile, err := runtimeProfile(opts.Runtime)
	if err != nil {
		return res, err
	}
	adapterRev := opts.AdapterRevision
	if adapterRev == "" {
		adapterRev = profile.Revision
	}
	pkgDigest, err := catalog.Digest(source)
	if err != nil {
		return res, err
	}

	plan, existingReg, existing, adapterMigration, err := planUpdateWithRegistry(profile.ID, scope, roots, pkg, pkgDigest, desired, adapterRev)
	res.Plan = plan
	res.AdapterMigration = adapterMigration
	if err != nil {
		return res, err
	}
	if plan.Conflicts != 0 {
		return res, apperr.Conflict("update_conflicts", fmt.Sprintf("update found %d conflict(s)", plan.Conflicts))
	}

	hasMutation := false
	for _, act := range plan.Actions {
		if act.Kind != "noop" {
			hasMutation = true
			break
		}
	}
	if !hasMutation && existing.Kit.Version == pkg.Version && existing.Kit.Digest == pkgDigest && existing.AdapterRevision == adapterRev && catalog.SelectionsEqual(existing.Selection, opts.Selection) {
		res.Noop = true
		res.Installation = *existing
		return res, nil
	}

	desiredByKey := map[string]adapter.Artifact{}
	for _, a := range desired {
		desiredByKey[a.Root+"\x00"+a.Path] = a
	}
	stateByKey := map[string]state.Artifact{}
	for _, sa := range existing.Artifacts {
		stateByKey[sa.Root+"\x00"+sa.Path] = sa
	}

	ordered := append([]planner.Action(nil), plan.Actions...)
	sort.Slice(ordered, func(i, k int) bool {
		if ordered[i].Root != ordered[k].Root {
			return ordered[i].Root < ordered[k].Root
		}
		return ordered[i].Path < ordered[k].Path
	})

	nextState := state.Installation{
		SchemaVersion:   existing.SchemaVersion,
		ManagedBy:       "dtkit",
		Generation:      existing.Generation + 1,
		Scope:           scope,
		Runtime:         profile.ID,
		AdapterRevision: adapterRev,
		CodexHome:       existing.CodexHome,
		RuntimeHome:     existing.RuntimeHome,
		Selection:       cloneSelection(opts.Selection),
		InstalledAt:     existing.InstalledAt,
		UpdatedAt:       now.UTC().Format(time.RFC3339),
		Kit: state.Kit{
			ID:            pkg.ID,
			Version:       pkg.Version,
			Digest:        pkgDigest,
			ArchiveSHA256: opts.ArchiveSHA256,
			SourceKind:    opts.SourceKind,
		},
		Hooks: append([]state.HookRegistration(nil), existing.Hooks...),
	}

	var txActions []TxAction
	for _, act := range ordered {
		key := act.Root + "\x00" + act.Path
		want, hasDesired := desiredByKey[key]
		recorded, hasRecorded := stateByKey[key]
		target, err := paths.ResolveTarget(roots[act.Root], act.Path)
		if err != nil {
			return res, err
		}

		var before []byte
		existed := false
		mode := os.FileMode(0644)
		if info, statErr := os.Lstat(target); statErr == nil {
			if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
				return res, apperr.WithPath(apperr.Conflict("target_not_regular", "target is not a regular file"), act.Path)
			}
			existed = true
			mode = info.Mode().Perm()
			before, err = readRegularLimited(target, maxArtifactBytes)
			if err != nil {
				return res, err
			}
		} else if !os.IsNotExist(statErr) {
			return res, statErr
		}

		if act.Kind == "noop" {
			if !hasRecorded {
				return res, fmt.Errorf("noop artifact missing from prior state: %s", act.Path)
			}
			nextState.Artifacts = append(nextState.Artifacts, recorded)
			continue
		}

		if act.Kind == "remove" {
			if existed {
				txActions = append(txActions, TxAction{
					Root:     act.Root,
					Path:     act.Path,
					Kind:     "remove",
					Strategy: recorded.Strategy,
					Existed:  true,
					Before:   before,
					Mode:     mode,
				})
			}
			continue
		}

		if !hasDesired {
			return res, fmt.Errorf("desired artifact missing for %s", act.Path)
		}

		after := append([]byte(nil), want.Content...)
		var nextBlock *planner.BlockState
		if want.Strategy == "managed-block" {
			if hasRecorded && existed {
				if recorded.Block == nil {
					return res, apperr.WithPath(apperr.Conflict("invalid_block_state", "missing managed block state"), act.Path)
				}
				var bs planner.BlockState
				after, bs, err = planner.ReplaceBlockForRuntime(profile.ID, pkg.ID, before, want.Content, scope, *recorded.Block)
				if err != nil {
					return res, apperr.WithPath(apperr.Conflict("managed_block_conflict", err.Error()), act.Path)
				}
				nextBlock = &bs
			} else {
				var bs planner.BlockState
				after, bs, err = planner.AppendBlockForRuntime(profile.ID, pkg.ID, before, existed, want.Content, scope)
				if err != nil {
					return res, apperr.WithPath(apperr.Conflict("managed_block_conflict", err.Error()), act.Path)
				}
				nextBlock = &bs
			}
		}

		itemDigest := contentHash(after)
		if nextBlock != nil {
			itemDigest = nextBlock.Digest
		}
		nextState.Artifacts = append(nextState.Artifacts, state.Artifact{
			Root:     want.Root,
			Path:     want.Path,
			Strategy: want.Strategy,
			Digest:   itemDigest,
			Block:    nextBlock,
		})
		txActions = append(txActions, TxAction{
			Root:     want.Root,
			Path:     want.Path,
			Kind:     act.Kind,
			Strategy: want.Strategy,
			Existed:  existed,
			Before:   before,
			After:    after,
			Mode:     mode,
		})
	}

	if adapterRev == profile.Revision {
		if err := nextState.Validate(desired); err != nil {
			return res, err
		}
	} else {
		if err := nextState.ValidateSelf(); err != nil {
			return res, err
		}
	}
	if scope == "global" {
		if profile.UsesCodexHome {
			nextState.CodexHome = resolveGlobalHomeFromRoots(profile, scope, roots)
		} else {
			nextState.RuntimeHome = resolveGlobalHomeFromRoots(profile, scope, roots)
		}
	}

	txReq := TxRequest{
		Runtime:   profile.ID,
		Operation: "update",
		Scope:     scope,
		KitID:     pkg.ID,
		Roots:     roots,
		Actions:   txActions,
		Now:       now,
		Hooks:     opts.Hooks,
	}
	if len(existingReg.Kits) >= 2 {
		nextReg := cloneRegistry(*existingReg)
		nextStateCopy := nextState
		nextStateCopy.Hooks = nil
		nextReg.Kits[pkg.ID] = nextStateCopy
		nextReg.UpdatedAt = nextStateCopy.UpdatedAt
		txReq.PrevRegistry = existingReg
		txReq.NextRegistry = &nextReg
	} else {
		txReq.PrevState = existing
		txReq.NextState = &nextState
	}
	txRes, err := ExecuteTx(txReq)
	if err != nil {
		return res, err
	}
	nextState.LastTransactionID = txRes.TransactionID
	res.Updated = true
	res.TransactionID = txRes.TransactionID
	res.Installation = nextState
	return res, nil
}

func compareSemver(a, b string) (int, error) {
	pa, err := parseSemver(a)
	if err != nil {
		return 0, err
	}
	pb, err := parseSemver(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < 3; i++ {
		if pa[i] < pb[i] {
			return -1, nil
		}
		if pa[i] > pb[i] {
			return 1, nil
		}
	}
	return 0, nil
}

func parseSemver(v string) ([3]int, error) {
	var out [3]int
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("invalid semver %q", v)
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, fmt.Errorf("invalid semver %q", v)
		}
		out[i] = n
	}
	return out, nil
}

func hasProjectOverridesDir(scope string, roots map[string]string, kitID string) bool {
	if scope != "project" || kitID == "" {
		return false
	}
	projRoot := roots["project"]
	if projRoot == "" {
		return false
	}
	overrideDir := filepath.Join(projRoot, ".dtkit", "overrides", kitID)
	info, err := os.Lstat(overrideDir)
	return err == nil && !paths.IsReparseOrSymlink(info) && info.IsDir()
}
