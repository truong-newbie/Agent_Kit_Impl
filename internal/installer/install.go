// Package installer applies previously validated DTKit plans.
package installer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/catalog"
	"dtkit.local/dtkit/internal/planner"
	"dtkit.local/dtkit/internal/state"
)

type journalFile struct {
	Root      string `json:"root"`
	Path      string `json:"path"`
	Existed   bool   `json:"existed"`
	Backup    string `json:"backup,omitempty"`
	AfterHash string `json:"afterHash,omitempty"`
}

type journal struct {
	SchemaVersion int           `json:"schemaVersion"`
	ID            string        `json:"id"`
	Phase         string        `json:"phase"`
	Scope         string        `json:"scope"`
	Files         []journalFile `json:"files"`
}

var artifactWriter = atomicWrite
var stateWriter = atomicWrite

// FreshInstall installs a package only when the provided preview is conflict-free.
// It re-plans under an exclusive DTKit lock before touching target artifacts.
func FreshInstall(scope string, roots map[string]string, pkg catalog.Package, source fs.FS, artifacts []codex.Artifact, now time.Time) (result state.Installation, err error) {
	plan, err := planner.InstallPreview(scope, roots, artifacts)
	if err != nil {
		return result, err
	}
	if plan.Conflicts != 0 {
		return result, fmt.Errorf("install has %d conflict(s)", plan.Conflicts)
	}
	stateRoot := "project"
	if scope == "global" {
		stateRoot = "user-home"
	}
	operational, err := planner.ResolveTarget(roots[stateRoot], ".dtkit/local")
	if err != nil {
		return result, err
	}
	if err := os.MkdirAll(operational, 0700); err != nil {
		return result, err
	}
	lock, err := acquireLock(filepath.Join(operational, "install.lock"))
	if err != nil {
		return result, err
	}
	defer func() {
		if releaseErr := lock.release(); err == nil && releaseErr != nil {
			err = releaseErr
		}
	}()
	// Re-plan after locking so preview assumptions cannot silently go stale.
	plan, err = planner.InstallPreview(scope, roots, artifacts)
	if err != nil {
		return result, err
	}
	if plan.Conflicts != 0 {
		return result, fmt.Errorf("install changed after preview")
	}
	id, err := randomID()
	if err != nil {
		return result, err
	}
	txDir := filepath.Join(operational, "transactions", id)
	if err := os.MkdirAll(txDir, 0700); err != nil {
		return result, err
	}
	j := journal{SchemaVersion: 1, ID: id, Phase: "preparing", Scope: scope}
	journalPath := filepath.Join(txDir, "journal.json")
	if err := writeJSON(journalPath, j, 0600); err != nil {
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
	prepared := make([]struct {
		action   planner.Action
		artifact codex.Artifact
		target   string
		before   []byte
		existed  bool
		mode     os.FileMode
		after    []byte
		block    *planner.BlockState
	}, 0, len(ordered))
	for index, action := range ordered {
		artifact, ok := byKey[action.Root+"\x00"+action.Path]
		if !ok {
			return result, fmt.Errorf("planned artifact disappeared")
		}
		target, err := planner.ResolveTarget(roots[action.Root], action.Path)
		if err != nil {
			return result, err
		}
		before, readErr := os.ReadFile(target)
		existed := readErr == nil
		if readErr != nil && !os.IsNotExist(readErr) {
			return result, readErr
		}
		mode := os.FileMode(0600)
		if existed {
			info, statErr := os.Stat(target)
			if statErr != nil {
				return result, statErr
			}
			mode = info.Mode().Perm()
		}
		after := append([]byte(nil), artifact.Content...)
		var block *planner.BlockState
		if artifact.Strategy == "managed-block" {
			var blockState planner.BlockState
			after, blockState, err = planner.AppendBlock(before, existed, artifact.Content, scope)
			if err != nil {
				return result, err
			}
			block = &blockState
		} else if existed {
			return result, fmt.Errorf("unowned target appeared: %s", action.Path)
		}
		backup := ""
		if existed {
			backup = fmt.Sprintf("before-%03d.bin", index)
			if err := os.WriteFile(filepath.Join(txDir, backup), before, 0600); err != nil {
				return result, err
			}
		}
		j.Files = append(j.Files, journalFile{Root: action.Root, Path: action.Path, Existed: existed, Backup: backup})
		prepared = append(prepared, struct {
			action   planner.Action
			artifact codex.Artifact
			target   string
			before   []byte
			existed  bool
			mode     os.FileMode
			after    []byte
			block    *planner.BlockState
		}{action, artifact, target, before, existed, mode, after, block})
	}
	j.Phase = "prepared"
	if err := writeJSON(journalPath, j, 0600); err != nil {
		return result, err
	}

	applied := 0
	defer func() {
		if err == nil {
			return
		}
		for i := applied - 1; i >= 0; i-- {
			item := prepared[i]
			current, readErr := os.ReadFile(item.target)
			if readErr != nil && !os.IsNotExist(readErr) {
				continue
			}
			if readErr == nil && contentHash(current) != contentHash(item.after) {
				continue
			}
			if item.existed {
				_ = atomicWrite(item.target, item.before, item.mode)
			} else if readErr == nil {
				_ = os.Remove(item.target)
			}
		}
		j.Phase = "rollback_attempted"
		_ = writeJSON(journalPath, j, 0600)
	}()

	result = state.Installation{SchemaVersion: 1, ManagedBy: "dtkit", Generation: 1, Scope: scope, Runtime: "codex", AdapterRevision: "1", InstalledAt: now.UTC().Format(time.RFC3339), UpdatedAt: now.UTC().Format(time.RFC3339), Kit: state.Kit{ID: pkg.ID, Version: pkg.Version}}
	if scope == "global" {
		result.CodexHome = roots["codex-home"]
	}
	result.Kit.Digest, err = catalog.Digest(source)
	if err != nil {
		return result, err
	}
	for i := range prepared {
		item := &prepared[i]
		if err = os.MkdirAll(filepath.Dir(item.target), 0755); err != nil {
			return result, err
		}
		if err = artifactWriter(item.target, item.after, item.mode); err != nil {
			return result, err
		}
		applied++
		hash := contentHash(item.after)
		itemHash := hash
		if item.block != nil {
			itemHash = item.block.Digest
		}
		result.Artifacts = append(result.Artifacts, state.Artifact{Root: item.artifact.Root, Path: item.artifact.Path, Strategy: item.artifact.Strategy, Digest: itemHash, Block: item.block})
		j.Files[i].AfterHash = hash
		j.Phase = "applying"
		if err = writeJSON(journalPath, j, 0600); err != nil {
			return result, err
		}
	}
	if err = result.Validate(artifacts); err != nil {
		return result, err
	}
	statePath, err := planner.ResolveTarget(roots[stateRoot], ".dtkit/state.json")
	if err != nil {
		return result, err
	}
	stateBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return result, err
	}
	stateBytes = append(stateBytes, '\n')
	if err = stateWriter(statePath, stateBytes, 0600); err != nil {
		return result, err
	}
	j.Phase = "committed"
	if err = writeJSON(journalPath, j, 0600); err != nil {
		return result, err
	}
	return result, nil
}

type lockFile struct{ path, token string }

func acquireLock(path string) (*lockFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	token, err := randomID()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("installation lock: %w", err)
	}
	if _, err = fmt.Fprintln(f, token); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return &lockFile{path: path, token: token}, nil
}

func (l *lockFile) release() error {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}
	if string(data) != l.token+"\n" {
		return fmt.Errorf("installation lock ownership changed")
	}
	return os.Remove(l.path)
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func contentHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func writeJSON(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), mode)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".dtkit-*.tmp")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err = temp.Chmod(mode); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tempName, path); err != nil {
		// Windows cannot rename over an existing file. The transaction journal
		// contains the preimage, so an in-place fallback remains recoverable.
		if _, statErr := os.Stat(path); statErr != nil {
			return err
		}
		file, openErr := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, mode)
		if openErr != nil {
			return err
		}
		_, writeErr := file.Write(data)
		if writeErr == nil {
			writeErr = file.Sync()
		}
		if closeErr := file.Close(); writeErr == nil {
			writeErr = closeErr
		}
		if writeErr != nil {
			return writeErr
		}
	}
	return nil
}
