// Package installer applies previously validated DTKit plans.
package installer

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

type InstallOptions struct {
	SourceKind string
	Hooks      *TxHooks
}

// FreshInstall installs a package only when the provided preview is conflict-free.
func FreshInstall(scope string, roots map[string]string, pkg catalog.Package, source fs.FS, artifacts []codex.Artifact, now time.Time) (state.Installation, error) {
	return FreshInstallWithOptions(scope, roots, pkg, source, artifacts, now, InstallOptions{})
}

func FreshInstallWithOptions(scope string, roots map[string]string, pkg catalog.Package, source fs.FS, artifacts []codex.Artifact, now time.Time, opts InstallOptions) (state.Installation, error) {
	var result state.Installation
	if now.IsZero() {
		now = time.Now()
	}
	plan, err := planner.InstallPreview(scope, roots, artifacts)
	if err != nil {
		return result, err
	}
	if plan.Conflicts != 0 {
		return result, apperr.Conflict("install_conflicts", fmt.Sprintf("install has %d conflict(s)", plan.Conflicts))
	}
	pending, lockStatus, err := InspectTransactions(scope, roots)
	if err != nil {
		return result, err
	}
	if len(pending) > 0 || lockStatus.Present {
		return result, apperr.Conflict("recovery_required", "unresolved transaction or lock exists; run dtkit recover first")
	}

	pkgDigest, err := catalog.Digest(source)
	if err != nil {
		return result, err
	}

	byKey := map[string]codex.Artifact{}
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
		Runtime:         "codex",
		AdapterRevision: codex.AdapterRevision,
		InstalledAt:     now.UTC().Format(time.RFC3339),
		UpdatedAt:       now.UTC().Format(time.RFC3339),
		Kit: state.Kit{
			ID:         pkg.ID,
			Version:    pkg.Version,
			Digest:     pkgDigest,
			SourceKind: opts.SourceKind,
		},
	}
	if scope == "global" {
		result.CodexHome = filepath.Clean(roots["codex-home"])
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
			after, blockState, err = planner.AppendBlockForKit(pkg.ID, before, existed, artifact.Content, scope)
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

	txRes, err := ExecuteTx(TxRequest{
		Operation: "install",
		Scope:     scope,
		KitID:     pkg.ID,
		Roots:     roots,
		Actions:   txActions,
		PrevState: nil,
		NextState: &result,
		Now:       now,
		Hooks:     opts.Hooks,
	})
	if err != nil {
		return result, err
	}
	result.LastTransactionID = txRes.TransactionID
	return result, nil
}
