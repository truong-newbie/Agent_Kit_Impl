package installer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

type lockMetadata struct {
	Token         string `json:"token"`
	PID           int    `json:"pid"`
	CreatedAt     string `json:"createdAt"`
	Operation     string `json:"operation"`
	TransactionID string `json:"transactionId,omitempty"`
}

type LockStatus struct {
	Present  bool         `json:"present"`
	Path     string       `json:"path"`
	Owner    lockMetadata `json:"owner,omitempty"`
	Liveness string       `json:"liveness,omitempty"` // "alive" | "dead" | "unknown"
	Corrupt  bool         `json:"corrupt,omitempty"`
}

type lockFile struct {
	path  string
	token string
}

func acquireLock(path, operation, txID string, now time.Time) (*lockFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	token, err := randomID()
	if err != nil {
		return nil, err
	}
	meta := lockMetadata{
		Token:         token,
		PID:           os.Getpid(),
		CreatedAt:     now.UTC().Format(time.RFC3339),
		Operation:     operation,
		TransactionID: txID,
	}
	payload, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil, apperr.WithPath(apperr.Conflict("lock_held", "another DTKit operation holds the installation lock"), path)
		}
		return nil, fmt.Errorf("installation lock: %w", err)
	}
	if _, err = f.Write(payload); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return nil, err
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return &lockFile{path: path, token: token}, nil
}

func (l *lockFile) release() error {
	if l == nil {
		return nil
	}
	data, err := os.ReadFile(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("installation lock disappeared before release")
		}
		return err
	}
	var meta lockMetadata
	if err := json.Unmarshal(data, &meta); err != nil || meta.Token != l.token {
		return fmt.Errorf("installation lock ownership changed")
	}
	return os.Remove(l.path)
}

func inspectLock(path string) (LockStatus, error) {
	status := LockStatus{Path: path}
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	status.Present = true
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() || info.Size() > 64*1024 {
		status.Corrupt = true
		status.Liveness = "unknown"
		return status, nil
	}
	data, err := readRegularLimited(path, 64*1024)
	if err != nil {
		return status, err
	}
	var meta lockMetadata
	if err := json.Unmarshal(data, &meta); err != nil || meta.Token == "" || meta.PID <= 0 {
		status.Corrupt = true
		status.Liveness = "unknown"
		return status, nil
	}
	status.Owner = meta
	status.Liveness = processLiveness(meta.PID)
	return status, nil
}

// canonicalTargetPath normalizes target (resolving symlinked existing ancestors and
// Windows casing) so aliases to the same physical file compute an identical lock key.
func canonicalTargetPath(target string) string {
	clean := filepath.Clean(target)
	if abs, err := filepath.Abs(clean); err == nil {
		clean = abs
	}
	var tail []string
	cur := clean
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			clean = filepath.Clean(resolved)
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
	if runtime.GOOS == "windows" {
		clean = strings.ToLower(clean)
	}
	return clean
}

type targetSpec struct {
	Root string
	Path string
}

func sharedTargetLockPaths(scope string, roots map[string]string, targets []targetSpec) ([]string, error) {
	baseRoot := roots[scopeRoot(scope)]
	if baseRoot == "" || len(targets) == 0 {
		return nil, nil
	}
	locksDir, err := paths.ResolveTarget(baseRoot, ".dtkit/locks")
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(targets))
	lockPaths := make([]string, 0, len(targets))
	for _, t := range targets {
		absTarget, rErr := paths.ResolveTarget(roots[t.Root], t.Path)
		if rErr != nil {
			return nil, rErr
		}
		canonical := canonicalTargetPath(absTarget)
		sum := sha256.Sum256([]byte(canonical))
		lp := filepath.Join(locksDir, hex.EncodeToString(sum[:16])+".lock")
		if _, ok := seen[lp]; !ok {
			seen[lp] = struct{}{}
			lockPaths = append(lockPaths, lp)
		}
	}
	sort.Strings(lockPaths)
	return lockPaths, nil
}

func acquireSharedLocks(lockPaths []string, operation, txID string, now time.Time) ([]*lockFile, error) {
	acquired := make([]*lockFile, 0, len(lockPaths))
	for _, lp := range lockPaths {
		l, err := acquireLock(lp, operation, txID, now)
		if err != nil {
			_ = releaseLockList(acquired)
			return nil, err
		}
		acquired = append(acquired, l)
	}
	return acquired, nil
}

func releaseLockList(locks []*lockFile) error {
	var firstErr error
	for i := len(locks) - 1; i >= 0; i-- {
		if err := locks[i].release(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func combineLockError(opErr, lockErr error) error {
	if lockErr == nil {
		return opErr
	}
	if opErr == nil {
		return lockErr
	}
	return errors.Join(opErr, lockErr)
}

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
