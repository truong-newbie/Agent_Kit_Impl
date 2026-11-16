package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

// TxHooks allows tests to inject deterministic faults at transaction boundaries
// without relying on global mutable state.
type TxHooks struct {
	BeforeBackup          func() error
	BeforePrepared        func() error
	BeforeAction          func(index int, action TxAction) error
	AfterAction           func(index int, action TxAction) error
	BeforeStateCommit     func() error
	BeforeCommitted       func() error
	BeforeRollbackRestore func(index int) error
}

type TxAction struct {
	Root     string
	Path     string
	Kind     string // create | update | remove
	Strategy string // whole-file | managed-block
	Existed  bool
	Before   []byte
	After    []byte
	Mode     os.FileMode
}

type TxRequest struct {
	Runtime   string // optional; defaults to codex
	Operation string // install | update | remove
	Scope     string // project | global
	KitID     string
	Roots     map[string]string
	Actions   []TxAction
	PrevState *state.Installation
	NextState *state.Installation
	Now       time.Time
	Hooks     *TxHooks
}

type TxResult struct {
	TransactionID string
	Journal       Journal
}

func ExecuteTx(req TxRequest) (res TxResult, err error) {
	if req.Scope != "project" && req.Scope != "global" {
		return res, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", req.Scope))
	}
	profile, pErr := runtimeProfile(req.Runtime)
	if pErr != nil {
		return res, pErr
	}
	if req.Now.IsZero() {
		req.Now = time.Now()
	}
	operational, err := runtimeOperationalTarget(profile.ID, req.Scope, req.Roots)
	if err != nil {
		return res, err
	}
	if err := os.MkdirAll(operational, 0700); err != nil {
		return res, err
	}

	txID, err := randomID()
	if err != nil {
		return res, err
	}
	lock, err := acquireLock(filepath.Join(operational, "install.lock"), req.Operation, txID, req.Now)
	if err != nil {
		return res, err
	}
	defer func() {
		if releaseErr := lock.release(); releaseErr != nil {
			if err == nil {
				err = releaseErr
			} else {
				err = fmt.Errorf("%w (lock release: %v)", err, releaseErr)
			}
		}
	}()

	txDir := filepath.Join(operational, "transactions", txID)
	if err := os.MkdirAll(txDir, 0700); err != nil {
		return res, err
	}
	nowStr := req.Now.UTC().Format(time.RFC3339)
	j := Journal{
		SchemaVersion: journalSchemaVer,
		ID:            txID,
		Operation:     req.Operation,
		Scope:         req.Scope,
		Runtime:       profile.ID,
		KitID:         req.KitID,
		Phase:         phasePreparing,
		CreatedAt:     nowStr,
		UpdatedAt:     nowStr,
	}
	if req.Scope == "global" {
		if profile.UsesCodexHome {
			j.CodexHome = resolveGlobalHomeFromRoots(profile, req.Scope, req.Roots)
		} else {
			j.RuntimeHome = resolveGlobalHomeFromRoots(profile, req.Scope, req.Roots)
		}
	}
	journalPath := filepath.Join(txDir, "journal.json")
	if err := writeJSON(journalPath, j, 0600); err != nil {
		return res, err
	}

	statePath, err := runtimeStateTarget(profile.ID, req.Scope, req.Roots)
	if err != nil {
		return res, err
	}

	// Re-verify state file matches expected PrevState under lock.
	var prevStateBytes []byte
	prevStateExists := false
	if existingState, readErr := readRegularLimited(statePath, maxStateBytes); readErr == nil {
		prevStateExists = true
		prevStateBytes = existingState
		if req.PrevState == nil {
			return res, apperr.WithPath(apperr.Conflict("state_appeared", "installation state appeared after preview"), statePath)
		}
		var currentState state.Installation
		if unmarshalErr := json.Unmarshal(existingState, &currentState); unmarshalErr != nil {
			return res, apperr.Wrap(apperr.ExitConflict, "invalid_state", "existing installation state is invalid", unmarshalErr)
		}
		if currentState.Generation != req.PrevState.Generation || currentState.Kit.Digest != req.PrevState.Kit.Digest {
			return res, apperr.Conflict("state_changed", "installation state changed after preview")
		}
	} else if !os.IsNotExist(readErr) {
		return res, readErr
	} else if req.PrevState != nil {
		return res, apperr.WithPath(apperr.Conflict("state_missing", "installation state disappeared after preview"), statePath)
	}

	if prevStateExists {
		j.PreviousState = JournalStateRef{
			Existed: true,
			Backup:  "state-before.bin",
			Digest:  contentHash(prevStateBytes),
		}
		if err := atomicWrite(filepath.Join(txDir, j.PreviousState.Backup), prevStateBytes, 0600); err != nil {
			return res, err
		}
	}

	var nextStateBytes []byte
	if req.NextState != nil {
		nextStateCopy := *req.NextState
		nextStateCopy.LastTransactionID = txID
		nextStateBytes, err = json.MarshalIndent(nextStateCopy, "", "  ")
		if err != nil {
			return res, err
		}
		nextStateBytes = append(nextStateBytes, '\n')
		j.NextState = JournalStateRef{
			Existed: true,
			Backup:  "state-after.bin",
			Digest:  contentHash(nextStateBytes),
		}
		if err := atomicWrite(filepath.Join(txDir, j.NextState.Backup), nextStateBytes, 0600); err != nil {
			return res, err
		}
	}

	if req.Hooks != nil && req.Hooks.BeforeBackup != nil {
		if err := req.Hooks.BeforeBackup(); err != nil {
			return res, err
		}
	}

	ordered := append([]TxAction(nil), req.Actions...)
	sort.Slice(ordered, func(i, k int) bool {
		if ordered[i].Root != ordered[k].Root {
			return ordered[i].Root < ordered[k].Root
		}
		return ordered[i].Path < ordered[k].Path
	})

	type preparedItem struct {
		action TxAction
		target string
	}
	prepared := make([]preparedItem, 0, len(ordered))
	for idx, act := range ordered {
		target, err := paths.ResolveTarget(req.Roots[act.Root], act.Path)
		if err != nil {
			return res, err
		}
		// Re-check disk right before preparing backups.
		diskInfo, statErr := os.Lstat(target)
		diskExists := statErr == nil
		if statErr != nil && !os.IsNotExist(statErr) {
			return res, statErr
		}
		if diskExists != act.Existed {
			return res, apperr.WithPath(apperr.Conflict("target_changed", "target existence changed after preview"), act.Path)
		}
		if diskExists {
			if paths.IsReparseOrSymlink(diskInfo) || !diskInfo.Mode().IsRegular() {
				return res, apperr.WithPath(apperr.Conflict("target_not_regular", "target is not a regular file"), act.Path)
			}
			cur, readErr := readRegularLimited(target, maxArtifactBytes)
			if readErr != nil {
				return res, readErr
			}
			if contentHash(cur) != contentHash(act.Before) {
				return res, apperr.WithPath(apperr.Conflict("target_changed", "target content changed after preview"), act.Path)
			}
		}

		ja := JournalAction{
			Root:     act.Root,
			Path:     act.Path,
			Kind:     act.Kind,
			Strategy: act.Strategy,
			Existed:  act.Existed,
			Mode:     uint32(act.Mode),
			Status:   "pending",
		}
		if act.Existed {
			ja.BeforeDigest = contentHash(act.Before)
			ja.Backup = fmt.Sprintf("before-%03d.bin", idx)
			if err := atomicWrite(filepath.Join(txDir, ja.Backup), act.Before, 0600); err != nil {
				return res, err
			}
		}
		if act.Kind == "remove" {
			ja.AfterMissing = true
		} else {
			ja.AfterDigest = contentHash(act.After)
			ja.AfterBackup = fmt.Sprintf("after-%03d.bin", idx)
			if err := atomicWrite(filepath.Join(txDir, ja.AfterBackup), act.After, 0600); err != nil {
				return res, err
			}
		}
		j.Actions = append(j.Actions, ja)
		prepared = append(prepared, preparedItem{action: act, target: target})
	}

	if req.Hooks != nil && req.Hooks.BeforePrepared != nil {
		if err := req.Hooks.BeforePrepared(); err != nil {
			return res, err
		}
	}
	j.Phase = phasePrepared
	j.UpdatedAt = req.Now.UTC().Format(time.RFC3339)
	if err := writeJSON(journalPath, j, 0600); err != nil {
		return res, err
	}

	stateCommitted := false
	seenDirs := map[string]bool{}

	defer func() {
		if err == nil {
			return
		}
		rbErr := rollbackTx(req.Roots, txDir, &j, statePath, stateCommitted, req.Hooks, req.Now)
		if rbErr != nil {
			err = fmt.Errorf("%w; %v", err, rbErr)
		}
	}()

	j.Phase = phaseApplying
	for i, item := range prepared {
		if req.Hooks != nil && req.Hooks.BeforeAction != nil {
			if err = req.Hooks.BeforeAction(i, item.action); err != nil {
				return res, err
			}
		}
		// Re-verify path containment and before-image immediately before mutating.
		target, resolveErr := paths.ResolveTarget(req.Roots[item.action.Root], item.action.Path)
		if resolveErr != nil {
			err = resolveErr
			return res, err
		}
		if err = verifyBeforeState(target, j.Actions[i]); err != nil {
			return res, err
		}

		if item.action.Kind == "remove" {
			if removeErr := os.Remove(target); removeErr != nil && !os.IsNotExist(removeErr) {
				err = removeErr
				return res, err
			}
		} else {
			if dirErr := ensureTrackedDirs(profile.ID, req.Scope, req.KitID, req.Roots[item.action.Root], item.action.Root, path.Dir(item.action.Path), &j.CreatedDirs, seenDirs); dirErr != nil {
				err = dirErr
				return res, err
			}
			mode := item.action.Mode
			if mode == 0 {
				mode = 0644
			}
			if writeErr := atomicWrite(target, item.action.After, mode); writeErr != nil {
				err = writeErr
				return res, err
			}
		}
		j.Actions[i].Status = "applied"
		j.UpdatedAt = req.Now.UTC().Format(time.RFC3339)
		if writeErr := writeJSON(journalPath, j, 0600); writeErr != nil {
			err = writeErr
			return res, err
		}
		if req.Hooks != nil && req.Hooks.AfterAction != nil {
			if err = req.Hooks.AfterAction(i, item.action); err != nil {
				return res, err
			}
		}
	}

	if req.Hooks != nil && req.Hooks.BeforeStateCommit != nil {
		if err = req.Hooks.BeforeStateCommit(); err != nil {
			return res, err
		}
	}

	if err = ensureTrackedDirs(profile.ID, req.Scope, req.KitID, req.Roots[scopeRoot(req.Scope)], scopeRoot(req.Scope), profile.StateDir(), &j.CreatedDirs, seenDirs); err != nil {
		return res, err
	}
	if req.NextState != nil {
		if err = atomicWrite(statePath, nextStateBytes, 0600); err != nil {
			return res, err
		}
	} else {
		if removeErr := os.Remove(statePath); removeErr != nil && !os.IsNotExist(removeErr) {
			err = removeErr
			return res, err
		}
	}
	stateCommitted = true

	if req.Operation == "remove" || req.Operation == "update" {
		pruneCreatedDirsForRemove(profile.ID, req.Scope, req.KitID, req.Roots, j.Actions)
	}

	if req.Hooks != nil && req.Hooks.BeforeCommitted != nil {
		if err = req.Hooks.BeforeCommitted(); err != nil {
			return res, err
		}
	}

	j.Phase = phaseCommitted
	j.UpdatedAt = req.Now.UTC().Format(time.RFC3339)
	if err = writeJSON(journalPath, j, 0600); err != nil {
		return res, err
	}
	_ = pruneOldTransactions(filepath.Join(operational, "transactions"))
	res = TxResult{TransactionID: txID, Journal: j}
	return res, nil
}

func verifyBeforeState(target string, ja JournalAction) error {
	info, err := os.Lstat(target)
	if !ja.Existed {
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		return apperr.WithPath(apperr.Conflict("unowned_target", "target file appeared concurrently"), ja.Path)
	}
	if err != nil {
		return err
	}
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return apperr.WithPath(apperr.Conflict("target_not_regular", "target is not a regular file"), ja.Path)
	}
	cur, err := readRegularLimited(target, maxArtifactBytes)
	if err != nil {
		return err
	}
	if contentHash(cur) != ja.BeforeDigest {
		return apperr.WithPath(apperr.Conflict("concurrent_modification", "target changed concurrently before write"), ja.Path)
	}
	return nil
}

func rollbackTx(roots map[string]string, txDir string, j *Journal, statePath string, stateCommitted bool, hooks *TxHooks, now time.Time) error {
	var rbErrors []string
	if stateCommitted {
		curState, readErr := readRegularLimited(statePath, maxStateBytes)
		canRestoreState := false
		if j.NextState.Existed {
			if readErr == nil && contentHash(curState) == j.NextState.Digest {
				canRestoreState = true
			}
		} else if os.IsNotExist(readErr) {
			canRestoreState = true
		}
		if !canRestoreState {
			rbErrors = append(rbErrors, "state.json changed concurrently during rollback")
		} else if j.PreviousState.Existed {
			backupPath, bErr := resolveBackupFile(txDir, j.PreviousState.Backup)
			if bErr != nil {
				rbErrors = append(rbErrors, fmt.Sprintf("read state backup: %v", bErr))
			} else if prevBytes, rErr := readRegularLimited(backupPath, maxStateBytes); rErr != nil {
				rbErrors = append(rbErrors, fmt.Sprintf("read state backup: %v", rErr))
			} else if wErr := atomicWrite(statePath, prevBytes, 0600); wErr != nil {
				rbErrors = append(rbErrors, fmt.Sprintf("restore state: %v", wErr))
			}
		} else if rmErr := os.Remove(statePath); rmErr != nil && !os.IsNotExist(rmErr) {
			rbErrors = append(rbErrors, fmt.Sprintf("remove new state: %v", rmErr))
		}
	}

	for i := len(j.Actions) - 1; i >= 0; i-- {
		act := &j.Actions[i]
		if act.Status != "applied" {
			continue
		}
		if hooks != nil && hooks.BeforeRollbackRestore != nil {
			if hookErr := hooks.BeforeRollbackRestore(i); hookErr != nil {
				act.Status = "conflict"
				rbErrors = append(rbErrors, fmt.Sprintf("%s: %v", act.Path, hookErr))
				continue
			}
		}
		target, err := paths.ResolveTarget(roots[act.Root], act.Path)
		if err != nil {
			act.Status = "conflict"
			rbErrors = append(rbErrors, fmt.Sprintf("%s: %v", act.Path, err))
			continue
		}
		cur, readErr := readRegularLimited(target, maxArtifactBytes)
		if act.AfterMissing {
			if !os.IsNotExist(readErr) {
				act.Status = "conflict"
				rbErrors = append(rbErrors, fmt.Sprintf("%s: recreated externally before rollback", act.Path))
				continue
			}
		} else {
			if readErr != nil || contentHash(cur) != act.AfterDigest {
				act.Status = "conflict"
				rbErrors = append(rbErrors, fmt.Sprintf("%s: modified externally before rollback", act.Path))
				continue
			}
		}

		if act.Existed {
			backupPath, bErr := resolveBackupFile(txDir, act.Backup)
			if bErr != nil {
				act.Status = "conflict"
				rbErrors = append(rbErrors, fmt.Sprintf("%s: %v", act.Path, bErr))
				continue
			}
			prevBytes, rErr := readRegularLimited(backupPath, maxArtifactBytes)
			if rErr != nil || contentHash(prevBytes) != act.BeforeDigest {
				act.Status = "conflict"
				rbErrors = append(rbErrors, fmt.Sprintf("%s: backup digest mismatch", act.Path))
				continue
			}
			mode := os.FileMode(act.Mode)
			if mode == 0 {
				mode = 0644
			}
			if wErr := atomicWrite(target, prevBytes, mode); wErr != nil {
				act.Status = "conflict"
				rbErrors = append(rbErrors, fmt.Sprintf("%s: %v", act.Path, wErr))
				continue
			}
		} else if readErr == nil {
			if rmErr := os.Remove(target); rmErr != nil && !os.IsNotExist(rmErr) {
				act.Status = "conflict"
				rbErrors = append(rbErrors, fmt.Sprintf("%s: %v", act.Path, rmErr))
				continue
			}
		}
		act.Status = "rolled_back"
	}

	pruneCreatedDirs(roots, j.CreatedDirs)
	if len(rbErrors) > 0 {
		j.Phase = phaseConflict
		j.UpdatedAt = now.UTC().Format(time.RFC3339)
		_ = writeJSON(filepath.Join(txDir, "journal.json"), j, 0600)
		return apperr.Conflict("rollback_conflict", strings.Join(rbErrors, "; "))
	}
	j.Phase = phaseRolledBack
	j.UpdatedAt = now.UTC().Format(time.RFC3339)
	if wErr := writeJSON(filepath.Join(txDir, "journal.json"), j, 0600); wErr != nil {
		return fmt.Errorf("write rolled_back journal: %w", wErr)
	}
	return nil
}

func ensureTrackedDirs(runtimeName, scope, kitID, absRoot, rootKey, relDir string, created *[]JournalDir, seen map[string]bool) error {
	if relDir == "" || relDir == "." {
		return nil
	}
	parts := strings.Split(relDir, "/")
	curRel := ""
	for _, seg := range parts {
		if curRel == "" {
			curRel = seg
		} else {
			curRel = curRel + "/" + seg
		}
		dirTarget, err := paths.ResolveTarget(absRoot, curRel)
		if err != nil {
			return err
		}
		info, statErr := os.Lstat(dirTarget)
		if statErr == nil {
			if paths.IsReparseOrSymlink(info) || !info.IsDir() {
				return apperr.WithPath(apperr.Conflict("invalid_directory", "expected directory in target path"), dirTarget)
			}
			continue
		}
		if !os.IsNotExist(statErr) {
			return statErr
		}
		if mkErr := os.Mkdir(dirTarget, 0755); mkErr != nil && !os.IsExist(mkErr) {
			return mkErr
		}
		key := rootKey + "\x00" + curRel
		if isAllowedCreatedDir(runtimeName, scope, kitID, rootKey, curRel) && !seen[key] {
			seen[key] = true
			*created = append(*created, JournalDir{Root: rootKey, Path: curRel})
		}
	}
	return nil
}

func pruneCreatedDirs(roots map[string]string, dirs []JournalDir) {
	for i := len(dirs) - 1; i >= 0; i-- {
		d := dirs[i]
		target, err := paths.ResolveTarget(roots[d.Root], d.Path)
		if err != nil {
			continue
		}
		entries, err := os.ReadDir(target)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(target)
		}
	}
}

func pruneCreatedDirsForRemove(runtimeName, scope, kitID string, roots map[string]string, actions []JournalAction) {
	seen := map[string]bool{}
	var candidates []JournalDir
	for _, act := range actions {
		if act.Kind != "remove" || act.Strategy != "whole-file" {
			continue
		}
		dir := path.Dir(act.Path)
		for dir != "." && dir != "" {
			if isAllowedCreatedDir(runtimeName, scope, kitID, act.Root, dir) {
				key := act.Root + "\x00" + dir
				if !seen[key] {
					seen[key] = true
					candidates = append(candidates, JournalDir{Root: act.Root, Path: dir})
				}
			}
			dir = path.Dir(dir)
		}
	}
	// Sort deepest paths first so child directories are removed before parents.
	sort.Slice(candidates, func(i, j int) bool {
		if len(candidates[i].Path) != len(candidates[j].Path) {
			return len(candidates[i].Path) > len(candidates[j].Path)
		}
		return candidates[i].Path > candidates[j].Path
	})
	for _, c := range candidates {
		target, err := paths.ResolveTarget(roots[c.Root], c.Path)
		if err != nil {
			continue
		}
		entries, err := os.ReadDir(target)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(target)
		}
	}
}

func pruneOldTransactions(txRoot string) error {
	entries, err := os.ReadDir(txRoot)
	if err != nil {
		return err
	}
	type completed struct {
		dir       string
		updatedAt string
	}
	var done []completed
	for _, entry := range entries {
		if !entry.IsDir() || !txIDPattern.MatchString(entry.Name()) {
			continue
		}
		jPath := filepath.Join(txRoot, entry.Name(), "journal.json")
		data, rErr := readRegularLimited(jPath, maxJournalBytes)
		if rErr != nil {
			continue
		}
		var j Journal
		if json.Unmarshal(data, &j) != nil || !j.IsTerminal() {
			continue
		}
		done = append(done, completed{dir: filepath.Join(txRoot, entry.Name()), updatedAt: j.UpdatedAt})
	}
	if len(done) <= maxRetainedTxs {
		return nil
	}
	sort.Slice(done, func(i, j int) bool {
		if done[i].updatedAt != done[j].updatedAt {
			return done[i].updatedAt > done[j].updatedAt
		}
		return done[i].dir > done[j].dir
	})
	for _, item := range done[maxRetainedTxs:] {
		_ = os.RemoveAll(item.dir)
	}
	return nil
}

func contentHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func writeJSON(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), mode)
}

// atomicWrite writes to a sibling temporary file, fsyncs, closes, and renames
// into place. It never truncates an existing target in place on rename failure.
func atomicWrite(target string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0755); err != nil {
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
	if err = os.Rename(tempName, target); err != nil {
		return errors.Join(fmt.Errorf("replace %s: %w", target, err))
	}
	return nil
}
