package check

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/project"
)

const (
	trustRelPath = ".dtkit/local/check-trust.json"
	lockRelPath  = ".dtkit/local/check.lock"
)

type TrustRecord struct {
	SchemaVersion int    `json:"schemaVersion"`
	TrustDigest   string `json:"trustDigest"`
	TrustedAt     string `json:"trustedAt"`
	CheckCount    int    `json:"checkCount"`
}

type TrustStatus struct {
	Trusted       bool   `json:"trusted"`
	CurrentDigest string `json:"currentDigest"`
	Recorded      string `json:"recordedDigest,omitempty"`
}

// InspectTrust compares the profile's canonical check command digest against recorded local trust.
func InspectTrust(repoRoot string, prof project.Profile) (TrustStatus, error) {
	current := project.CommandsTrustDigest(prof)
	target, err := paths.ResolveTarget(repoRoot, trustRelPath)
	if err != nil {
		return TrustStatus{}, err
	}
	info, statErr := os.Lstat(target)
	if os.IsNotExist(statErr) {
		return TrustStatus{Trusted: false, CurrentDigest: current}, nil
	}
	if statErr != nil {
		return TrustStatus{}, statErr
	}
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return TrustStatus{}, apperr.WithPath(apperr.Conflict("invalid_trust_file", "check-trust.json is not a regular file"), trustRelPath)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		return TrustStatus{}, err
	}
	var rec TrustRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return TrustStatus{Trusted: false, CurrentDigest: current}, nil
	}
	return TrustStatus{
		Trusted:       rec.TrustDigest != "" && rec.TrustDigest == current,
		CurrentDigest: current,
		Recorded:      rec.TrustDigest,
	}, nil
}

// RecordTrust atomically persists trust confirmation for the current check command digest.
func RecordTrust(repoRoot string, prof project.Profile, now time.Time) error {
	target, err := paths.ResolveTarget(repoRoot, trustRelPath)
	if err != nil {
		return err
	}
	if now.IsZero() {
		now = time.Now()
	}
	rec := TrustRecord{
		SchemaVersion: 1,
		TrustDigest:   project.CommandsTrustDigest(prof),
		TrustedAt:     now.UTC().Format(time.RFC3339),
		CheckCount:    len(prof.Checks),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".check-trust.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		return err
	}
	cleanup = false
	return nil
}

type lockMeta struct {
	PID       int    `json:"pid"`
	CreatedAt string `json:"createdAt"`
}

func acquireCheckLock(repoRoot string, now time.Time) (func(), error) {
	target, err := paths.ResolveTarget(repoRoot, lockRelPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(lockMeta{PID: os.Getpid(), CreatedAt: now.UTC().Format(time.RFC3339)})
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil, apperr.WithPath(
				apperr.Conflict("check_lock_held", fmt.Sprintf("another dtkit check run holds %s", lockRelPath)),
				lockRelPath,
			)
		}
		return nil, err
	}
	_, _ = f.Write(payload)
	_ = f.Close()
	return func() {
		_ = os.Remove(target)
	}, nil
}
