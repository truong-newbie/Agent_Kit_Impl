package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

type PendingTx struct {
	ID        string `json:"id"`
	Operation string `json:"operation"`
	Runtime   string `json:"runtime,omitempty"`
	Scope     string `json:"scope"`
	Phase     string `json:"phase"`
	CreatedAt string `json:"createdAt"`
	Detail    string `json:"detail,omitempty"`
}

type RecoverReport struct {
	Runtime       string      `json:"runtime,omitempty"`
	Scope         string      `json:"scope"`
	Outcome       string      `json:"outcome"` // noop | committed | rolled_back | conflict
	TransactionID string      `json:"transactionId,omitempty"`
	Pending       []PendingTx `json:"pending,omitempty"`
	Lock          *LockStatus `json:"lock,omitempty"`
	Detail        string      `json:"detail,omitempty"`
}

// InspectTransactions returns unresolved Codex journals and current lock status without writing.
func InspectTransactions(scope string, roots map[string]string) ([]PendingTx, LockStatus, error) {
	return InspectTransactionsForRuntime(adapter.DefaultRuntime, scope, roots)
}

// InspectTransactionsForRuntime returns unresolved journals and lock status for the named runtime.
func InspectTransactionsForRuntime(runtimeName, scope string, roots map[string]string) ([]PendingTx, LockStatus, error) {
	var lockStatus LockStatus
	profile, err := runtimeProfile(runtimeName)
	if err != nil {
		return nil, lockStatus, err
	}
	operational, err := runtimeOperationalTarget(profile.ID, scope, roots)
	if err != nil {
		return nil, lockStatus, err
	}
	lockStatus, err = inspectLock(filepath.Join(operational, "install.lock"))
	if err != nil {
		return nil, lockStatus, err
	}

	txRoot := filepath.Join(operational, "transactions")
	info, err := os.Lstat(txRoot)
	if os.IsNotExist(err) {
		return nil, lockStatus, nil
	}
	if err != nil {
		return nil, lockStatus, err
	}
	if paths.IsReparseOrSymlink(info) || !info.IsDir() {
		return nil, lockStatus, apperr.WithPath(apperr.Conflict("invalid_transactions_dir", "transactions path is not a regular directory"), txRoot)
	}
	entries, err := os.ReadDir(txRoot)
	if err != nil {
		return nil, lockStatus, err
	}
	var pending []PendingTx
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		jPath := filepath.Join(txRoot, id, "journal.json")
		jInfo, statErr := os.Lstat(jPath)
		if os.IsNotExist(statErr) {
			pending = append(pending, PendingTx{ID: id, Runtime: profile.ID, Scope: scope, Phase: "corrupt", Detail: "missing journal.json"})
			continue
		}
		if statErr != nil {
			return nil, lockStatus, statErr
		}
		if paths.IsReparseOrSymlink(jInfo) || !jInfo.Mode().IsRegular() || jInfo.Size() > maxJournalBytes {
			pending = append(pending, PendingTx{ID: id, Runtime: profile.ID, Scope: scope, Phase: "corrupt", Detail: "invalid or oversized journal.json"})
			continue
		}
		data, rErr := readRegularLimited(jPath, maxJournalBytes)
		if rErr != nil {
			pending = append(pending, PendingTx{ID: id, Runtime: profile.ID, Scope: scope, Phase: "corrupt", Detail: rErr.Error()})
			continue
		}
		var j Journal
		if uErr := json.Unmarshal(data, &j); uErr != nil {
			pending = append(pending, PendingTx{ID: id, Runtime: profile.ID, Scope: scope, Phase: "corrupt", Detail: uErr.Error()})
			continue
		}
		if vErr := validateJournalForRuntime(j, scope, profile.ID); vErr != nil {
			pending = append(pending, PendingTx{ID: id, Runtime: profile.ID, Scope: scope, Phase: "corrupt", Detail: vErr.Error()})
			continue
		}
		if !j.IsTerminal() {
			pending = append(pending, PendingTx{
				ID:        j.ID,
				Operation: j.Operation,
				Runtime:   j.EffectiveRuntime(),
				Scope:     j.Scope,
				Phase:     j.Phase,
				CreatedAt: j.CreatedAt,
			})
		}
	}
	sort.Slice(pending, func(i, j int) bool { return pending[i].ID < pending[j].ID })
	return pending, lockStatus, nil
}

// Recover inspects and resolves an interrupted Codex DTKit transaction.
func Recover(scope string, roots map[string]string, txID string, dryRun bool, now time.Time) (RecoverReport, error) {
	return RecoverForRuntime(adapter.DefaultRuntime, scope, roots, txID, dryRun, now)
}

// RecoverForRuntime inspects and resolves an interrupted DTKit transaction for runtimeName.
func RecoverForRuntime(runtimeName, scope string, roots map[string]string, txID string, dryRun bool, now time.Time) (report RecoverReport, err error) {
	profile, pErr := runtimeProfile(runtimeName)
	report = RecoverReport{Runtime: adapter.NormalizeRuntime(runtimeName), Scope: scope}
	if pErr != nil {
		return report, pErr
	}
	report.Runtime = profile.ID
	if !profile.SupportsScope(scope) {
		return report, apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	if now.IsZero() {
		now = time.Now()
	}

	pending, lockStatus, err := InspectTransactionsForRuntime(profile.ID, scope, roots)
	if err != nil {
		return report, err
	}
	report.Pending = pending
	if lockStatus.Present {
		report.Lock = &lockStatus
	}

	if lockStatus.Present {
		if lockStatus.Corrupt || lockStatus.Liveness != "dead" {
			return report, apperr.WithPath(apperr.Conflict("active_or_uncertain_lock", fmt.Sprintf("installation lock is held or liveness is %s; cannot safely clear", lockStatus.Liveness)), lockStatus.Path)
		}
	}

	if len(pending) == 0 {
		if lockStatus.Present && lockStatus.Liveness == "dead" {
			if dryRun {
				report.Outcome = phaseRolledBack
				report.Detail = "stale lock from dead process would be cleared"
				return report, nil
			}
			clearDeadOwnedSharedLocks(scope, roots, lockStatus.Owner.TransactionID, lockStatus.Owner.PID)
			if rmErr := os.Remove(lockStatus.Path); rmErr != nil && !os.IsNotExist(rmErr) {
				return report, rmErr
			}
			report.Outcome = phaseRolledBack
			report.Detail = "cleared stale installation lock from dead process"
			return report, nil
		}
		report.Outcome = "noop"
		report.Detail = "no unfinished transactions found"
		return report, nil
	}

	var targetTx PendingTx
	if txID == "" {
		if len(pending) > 1 {
			ids := make([]string, 0, len(pending))
			for _, p := range pending {
				ids = append(ids, p.ID)
			}
			return report, apperr.WithHint(apperr.Conflict("multiple_pending_transactions", fmt.Sprintf("multiple unfinished transactions found: %s", strings.Join(ids, ", "))), "re-run with --transaction <id>")
		}
		targetTx = pending[0]
	} else {
		found := false
		for _, p := range pending {
			if p.ID == txID {
				targetTx = p
				found = true
				break
			}
		}
		if !found {
			return report, apperr.Usage("transaction_not_found", fmt.Sprintf("unfinished transaction %q not found", txID))
		}
	}
	report.TransactionID = targetTx.ID

	operational, err := runtimeOperationalTarget(profile.ID, scope, roots)
	if err != nil {
		return report, err
	}
	txDir := filepath.Join(operational, "transactions", targetTx.ID)
	jPath := filepath.Join(txDir, "journal.json")
	data, err := readRegularLimited(jPath, maxJournalBytes)
	if err != nil {
		return report, apperr.Wrap(apperr.ExitConflict, "corrupt_journal", fmt.Sprintf("cannot read journal %s: %v", targetTx.ID, err), err)
	}
	var j Journal
	if err := json.Unmarshal(data, &j); err != nil {
		return report, apperr.Wrap(apperr.ExitConflict, "corrupt_journal", fmt.Sprintf("invalid journal %s: %v", targetTx.ID, err), err)
	}
	if err := validateJournalForRuntime(j, scope, profile.ID); err != nil {
		return report, apperr.Wrap(apperr.ExitConflict, "invalid_journal", err.Error(), err)
	}
	if err := checkGlobalHomeBinding(profile, scope, j.GlobalHome(), roots); err != nil {
		return report, err
	}

	statePath, err := runtimeStateTarget(profile.ID, scope, roots)
	if err != nil {
		return report, err
	}

	specs := make([]targetSpec, 0, len(j.Actions))
	for _, act := range j.Actions {
		specs = append(specs, targetSpec{Root: act.Root, Path: act.Path})
	}
	sharedLockPaths, err := sharedTargetLockPaths(scope, roots, specs)
	if err != nil {
		return report, err
	}
	var deadOwnedSharedLocks []string
	for _, lp := range sharedLockPaths {
		sl, lErr := inspectLock(lp)
		if lErr != nil {
			return report, lErr
		}
		if sl.Present {
			if !sl.Corrupt && sl.Liveness == "dead" && (sl.Owner.TransactionID == j.ID || (lockStatus.Present && lockStatus.Owner.PID == sl.Owner.PID)) {
				deadOwnedSharedLocks = append(deadOwnedSharedLocks, lp)
				continue
			}
			return report, apperr.WithPath(apperr.Conflict("active_or_uncertain_lock", fmt.Sprintf("shared target lock is held or liveness is %s; cannot safely recover", sl.Liveness)), lp)
		}
	}

	if dryRun {
		allAfter, canRollback, conflicts, err := evaluateTxOnDisk(roots, txDir, j, statePath)
		if err != nil {
			return report, err
		}
		if len(conflicts) > 0 {
			report.Outcome = phaseConflict
			report.Detail = strings.Join(conflicts, "; ")
			return report, apperr.Conflict("recovery_conflict", report.Detail)
		}
		if allAfter {
			report.Outcome = phaseCommitted
			report.Detail = "all targets and state already match committed after-image"
			return report, nil
		}
		if !canRollback {
			report.Outcome = phaseConflict
			report.Detail = "cannot safely determine before or after state for recovery"
			return report, apperr.Conflict("recovery_conflict", report.Detail)
		}
		report.Outcome = phaseRolledBack
		report.Detail = "transaction can be cleanly rolled back to pre-transaction state"
		return report, nil
	}

	if lockStatus.Present && lockStatus.Liveness == "dead" {
		if rmErr := os.Remove(lockStatus.Path); rmErr != nil && !os.IsNotExist(rmErr) {
			return report, rmErr
		}
	}
	lock, err := acquireLock(filepath.Join(operational, "install.lock"), "recover", j.ID, now)
	if err != nil {
		return report, err
	}
	defer func() {
		err = combineLockError(err, lock.release())
	}()

	for _, lp := range deadOwnedSharedLocks {
		if rmErr := os.Remove(lp); rmErr != nil && !os.IsNotExist(rmErr) {
			return report, rmErr
		}
	}
	sharedLocks, err := acquireSharedLocks(sharedLockPaths, "recover", j.ID, now)
	if err != nil {
		return report, err
	}
	defer func() {
		err = combineLockError(err, releaseLockList(sharedLocks))
	}()

	// Evaluate disk state strictly under full runtime + shared target lock protection.
	allAfter, canRollback, conflicts, err := evaluateTxOnDisk(roots, txDir, j, statePath)
	if err != nil {
		return report, err
	}
	if len(conflicts) > 0 {
		report.Outcome = phaseConflict
		report.Detail = strings.Join(conflicts, "; ")
		j.Phase = phaseConflict
		j.UpdatedAt = now.UTC().Format(time.RFC3339)
		_ = writeJSON(jPath, j, 0600)
		return report, apperr.Conflict("recovery_conflict", report.Detail)
	}

	if allAfter {
		report.Outcome = phaseCommitted
		report.Detail = "all targets and state already match committed after-image"
		for i := range j.Actions {
			j.Actions[i].Status = "applied"
		}
		j.Phase = phaseCommitted
		j.UpdatedAt = now.UTC().Format(time.RFC3339)
		if err := writeJSON(jPath, j, 0600); err != nil {
			return report, err
		}
		_ = pruneOldTransactions(filepath.Join(operational, "transactions"))
		return report, nil
	}

	if !canRollback {
		report.Outcome = phaseConflict
		report.Detail = "cannot safely determine before or after state for recovery"
		return report, apperr.Conflict("recovery_conflict", report.Detail)
	}

	if err := executeRecoveryRollback(roots, txDir, &j, statePath, now); err != nil {
		report.Outcome = phaseConflict
		report.Detail = err.Error()
		return report, err
	}
	report.Outcome = phaseRolledBack
	report.Detail = "rolled back unfinished transaction to pre-transaction state"
	_ = pruneOldTransactions(filepath.Join(operational, "transactions"))
	return report, nil
}

func clearDeadOwnedSharedLocks(scope string, roots map[string]string, txID string, pid int) {
	baseRoot := roots[scopeRoot(scope)]
	if baseRoot == "" || (txID == "" && pid <= 0) {
		return
	}
	locksDir, err := paths.ResolveTarget(baseRoot, ".dtkit/locks")
	if err != nil {
		return
	}
	entries, err := os.ReadDir(locksDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lock") {
			continue
		}
		lp := filepath.Join(locksDir, entry.Name())
		st, lErr := inspectLock(lp)
		if lErr != nil || !st.Present || st.Corrupt || st.Liveness != "dead" {
			continue
		}
		if (txID != "" && st.Owner.TransactionID == txID) || (txID == "" && pid > 0 && st.Owner.PID == pid) {
			_ = os.Remove(lp)
		}
	}
}

func evaluateTxOnDisk(roots map[string]string, txDir string, j Journal, statePath string) (allAfter bool, canRollback bool, conflicts []string, err error) {
	allAfter = j.Phase != phasePreparing
	canRollback = true

	// Check state.json vs PreviousState / NextState.
	curState, sErr := readRegularLimited(statePath, maxStateBytes)
	stateExists := sErr == nil
	if sErr != nil && !os.IsNotExist(sErr) {
		return false, false, nil, sErr
	}
	matchesPrevState := (!j.PreviousState.Existed && !stateExists) || (j.PreviousState.Existed && stateExists && contentHash(curState) == j.PreviousState.Digest)
	matchesNextState := (!j.NextState.Existed && !stateExists) || (j.NextState.Existed && stateExists && contentHash(curState) == j.NextState.Digest)
	if !matchesNextState {
		allAfter = false
	}
	if !matchesPrevState && !matchesNextState {
		canRollback = false
		conflicts = append(conflicts, "state.json matches neither before nor after transaction image")
	}

	for _, act := range j.Actions {
		target, rErr := paths.ResolveTarget(roots[act.Root], act.Path)
		if rErr != nil {
			return false, false, nil, rErr
		}
		info, lErr := os.Lstat(target)
		exists := lErr == nil
		if lErr != nil && !os.IsNotExist(lErr) {
			return false, false, nil, lErr
		}
		var diskHash string
		if exists {
			if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
				conflicts = append(conflicts, fmt.Sprintf("%s: target is not a regular file", act.Path))
				canRollback = false
				allAfter = false
				continue
			}
			b, readErr := readRegularLimited(target, maxArtifactBytes)
			if readErr != nil {
				return false, false, nil, readErr
			}
			diskHash = contentHash(b)
		}

		matchesBefore := (!act.Existed && !exists) || (act.Existed && exists && diskHash == act.BeforeDigest)
		matchesAfter := (act.AfterMissing && !exists) || (!act.AfterMissing && exists && diskHash == act.AfterDigest)
		if !matchesAfter {
			allAfter = false
		}
		if !matchesBefore && !matchesAfter {
			canRollback = false
			conflicts = append(conflicts, fmt.Sprintf("%s: on-disk content matches neither before nor after image", act.Path))
		}
		if matchesAfter && !matchesBefore {
			if act.Existed {
				if _, bErr := resolveBackupFile(txDir, act.Backup); bErr != nil {
					canRollback = false
					conflicts = append(conflicts, fmt.Sprintf("%s: missing preimage backup (%v)", act.Path, bErr))
				}
			}
		}
	}
	return allAfter, canRollback, conflicts, nil
}

func executeRecoveryRollback(roots map[string]string, txDir string, j *Journal, statePath string, now time.Time) error {
	// Mark any actions currently at after-state as applied so rollbackTx restores them cleanly.
	curState, sErr := readRegularLimited(statePath, maxStateBytes)
	stateAtNext := (!j.NextState.Existed && os.IsNotExist(sErr)) || (j.NextState.Existed && sErr == nil && contentHash(curState) == j.NextState.Digest)
	stateAtPrev := (!j.PreviousState.Existed && os.IsNotExist(sErr)) || (j.PreviousState.Existed && sErr == nil && contentHash(curState) == j.PreviousState.Digest)
	stateCommitted := stateAtNext && !stateAtPrev

	for i := range j.Actions {
		act := &j.Actions[i]
		target, err := paths.ResolveTarget(roots[act.Root], act.Path)
		if err != nil {
			return err
		}
		cur, rErr := readRegularLimited(target, maxArtifactBytes)
		exists := rErr == nil
		matchesBefore := (!act.Existed && os.IsNotExist(rErr)) || (act.Existed && exists && contentHash(cur) == act.BeforeDigest)
		matchesAfter := (act.AfterMissing && os.IsNotExist(rErr)) || (!act.AfterMissing && exists && contentHash(cur) == act.AfterDigest)
		if matchesAfter && !matchesBefore {
			act.Status = "applied"
		} else {
			act.Status = "pending"
		}
	}
	return rollbackTx(roots, txDir, j, statePath, stateCommitted, nil, now)
}
