package installer

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dtkit.local/dtkit/internal/apperr"
	"dtkit.local/dtkit/internal/paths"
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

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
