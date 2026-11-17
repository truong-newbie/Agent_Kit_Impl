// Package installer applies previously validated DTKit plans.
package installer

import (
	"fmt"
	"io/fs"
	"os"
	"sort"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

type InstallOptions struct {
	Runtime       string // optional; defaults to codex
	SourceKind    string
	ArchiveSHA256 string
	Selection     *catalog.SelectionSpec
	Hooks         *TxHooks
}

// FreshInstall installs a Codex package only when the provided preview is conflict-free.
func FreshInstall(scope string, roots map[string]string, pkg catalog.Package, source fs.FS, artifacts []adapter.Artifact, now time.Time) (state.Installation, error) {
	return FreshInstallWithOptions(scope, roots, pkg, source, artifacts, now, InstallOptions{})
}

func FreshInstallWithOptions(scope string, roots map[string]string, pkg catalog.Package, source fs.FS, artifacts []adapter.Artifact, now time.Time, opts InstallOptions) (state.Installation, error) {
	var result state.Installation
	if now.IsZero() {
		now = time.Now()
	}
	profile, err := runtimeProfile(opts.Runtime)
	if err != nil {
		return result, err
	}
	pending, lockStatus, err := InspectTransactionsForRuntime(profile.ID, scope, roots)
	if err != nil {
		return result, err
	}
	if len(pending) > 0 || lockStatus.Present {
		return result, apperr.Conflict("recovery_required", "unresolved transaction or lock exists; run dtkit recover first")
	}

	statePath, err := runtimeStateTarget(profile.ID, scope, roots)
	if err != nil {
		return result, err
	}
	existingReg, regExists, err := state.LoadRegistryForRuntime(statePath, scope, profile.ID)
	if err != nil {
		return result, err
	}
	var peerWhole map[string]string
	if regExists {
		if err := checkGlobalHomeBinding(profile, scope, existingReg.GlobalHome(), roots); err != nil {
			return result, err
		}
		if prevInst, exists := existingReg.Kits[pkg.ID]; exists {
			return result, apperr.WithHint(apperr.Conflict("already_installed", fmt.Sprintf("%s %s is already installed in %s scope (%s)", pkg.ID, prevInst.Kit.Version, scope, profile.ID)), fmt.Sprintf("run dtkit update %s --%s --runtime %s", pkg.ID, scope, profile.ID))
		}
		peerWhole = existingReg.PeerOwnedWholeFiles(pkg.ID)
	}

	plan, err := planner.PlanDesiredWithPeersForRuntime(profile.ID, scope, pkg.ID, roots, nil, peerWhole, artifacts)
	if err != nil {
		return result, err
	}
	if plan.Conflicts != 0 {
		return result, apperr.Conflict("install_conflicts", fmt.Sprintf("install has %d conflict(s)", plan.Conflicts))
	}

	pkgDigest, err := catalog.Digest(source)
	if err != nil {
		return result, err
	}

	byKey := map[string]adapter.Artifact{}
	for _, artifact := range artifacts {
		byKey[artifact.Root+"\x00"+artifact.Path] = artifact
	}
	ordered := append([]planner.Action(nil), plan.Actions...)
	sort.Slice(ordered, func(i, k int) bool {
		if ordered[i].Root != ordered[k].Root {
			return ordered[i].Root < ordered[k].Root
		}
		return ordered[i].Path < ordered[k].Path
	})

	result = state.Installation{
		SchemaVersion:   1,
		ManagedBy:       "dtkit",
		Generation:      1,
		Scope:           scope,
		Runtime:         profile.ID,
		AdapterRevision: profile.Revision,
		Selection:       cloneSelection(opts.Selection),
		InstalledAt:     now.UTC().Format(time.RFC3339),
		UpdatedAt:       now.UTC().Format(time.RFC3339),
		Kit: state.Kit{
			ID:            pkg.ID,
			Version:       pkg.Version,
			Digest:        pkgDigest,
			ArchiveSHA256: opts.ArchiveSHA256,
			SourceKind:    opts.SourceKind,
		},
	}
	if scope == "global" {
		if profile.UsesCodexHome {
			result.CodexHome = resolveGlobalHomeFromRoots(profile, scope, roots)
		} else {
			result.RuntimeHome = resolveGlobalHomeFromRoots(profile, scope, roots)
		}
	}

	txActions := make([]TxAction, 0, len(ordered))
	for _, action := range ordered {
		artifact, ok := byKey[action.Root+"\x00"+action.Path]
		if !ok {
			return result, fmt.Errorf("planned artifact disappeared")
		}
		target, err := paths.ResolveTarget(roots[action.Root], action.Path)
		if err != nil {
			return result, err
		}
		var before []byte
		existed := false
		mode := os.FileMode(0644)
		if info, statErr := os.Lstat(target); statErr == nil {
			if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
				return result, apperr.WithPath(apperr.Conflict("target_not_regular", "target is not a regular file"), action.Path)
			}
			existed = true
			mode = info.Mode().Perm()
			before, err = readRegularLimited(target, maxArtifactBytes)
			if err != nil {
				return result, err
			}
		} else if !os.IsNotExist(statErr) {
			return result, statErr
		}

		after := append([]byte(nil), artifact.Content...)
		var block *planner.BlockState
		if artifact.Strategy == "managed-block" {
			var blockState planner.BlockState
			after, blockState, err = planner.AppendBlockForRuntime(profile.ID, pkg.ID, before, existed, artifact.Content, scope)
			if err != nil {
				return result, apperr.WithPath(apperr.Conflict("managed_block_conflict", err.Error()), action.Path)
			}
			block = &blockState
		} else if existed {
			return result, apperr.WithPath(apperr.Conflict("unowned_target", fmt.Sprintf("unowned target appeared: %s", action.Path)), action.Path)
		}

		itemDigest := contentHash(after)
		if block != nil {
			itemDigest = block.Digest
		}
		result.Artifacts = append(result.Artifacts, state.Artifact{
			Root:     artifact.Root,
			Path:     artifact.Path,
			Strategy: artifact.Strategy,
			Digest:   itemDigest,
			Block:    block,
		})
		txActions = append(txActions, TxAction{
			Root:     artifact.Root,
			Path:     artifact.Path,
			Kind:     action.Kind,
			Strategy: artifact.Strategy,
			Existed:  existed,
			Before:   before,
			After:    after,
			Mode:     mode,
		})
	}

	if err := result.Validate(artifacts); err != nil {
		return result, err
	}

	txReq := TxRequest{
		Runtime:   profile.ID,
		Operation: "install",
		Scope:     scope,
		KitID:     pkg.ID,
		Roots:     roots,
		Actions:   txActions,
		Now:       now,
		Hooks:     opts.Hooks,
	}
	if regExists {
		nextReg := cloneRegistry(*existingReg)
		nextReg.Kits[pkg.ID] = result
		nextReg.UpdatedAt = result.UpdatedAt
		txReq.PrevRegistry = existingReg
		txReq.NextRegistry = &nextReg
	} else {
		txReq.PrevState = nil
		txReq.NextState = &result
	}
	txRes, err := ExecuteTx(txReq)
	if err != nil {
		return result, err
	}
	result.LastTransactionID = txRes.TransactionID
	return result, nil
}

func cloneRegistry(r state.Registry) state.Registry {
	out := r
	out.Kits = make(map[string]state.Installation, len(r.Kits))
	for k, v := range r.Kits {
		out.Kits[k] = v
	}
	if len(r.Hooks) > 0 {
		out.Hooks = append([]state.HookRegistration(nil), r.Hooks...)
	}
	return out
}

func cloneSelection(s *catalog.SelectionSpec) *catalog.SelectionSpec {
	if s == nil {
		return nil
	}
	out := &catalog.SelectionSpec{Preset: s.Preset}
	if len(s.RequestedComponents) > 0 {
		out.RequestedComponents = append([]string{}, s.RequestedComponents...)
	}
	if len(s.ResolvedComponents) > 0 {
		out.ResolvedComponents = append([]string{}, s.ResolvedComponents...)
	}
	return out
}
