package guard

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// Receipt proves that configured project checks passed against a specific workspace state digest.
type Receipt struct {
	SchemaVersion   int      `json:"schemaVersion"`
	WorkspaceDigest string   `json:"workspaceDigest"`
	FileCount       int      `json:"fileCount"`
	PassedChecks    []string `json:"passedChecks"`
	VerifiedAt      string   `json:"verifiedAt"`
}

// Streak tracks consecutive failing check runs to surface retry-loop warnings.
type Streak struct {
	SchemaVersion       int      `json:"schemaVersion"`
	ConsecutiveFailures int      `json:"consecutiveFailures"`
	LastFailedChecks    []string `json:"lastFailedChecks,omitempty"`
	UpdatedAt           string   `json:"updatedAt,omitempty"`
}

// VerificationStatus combines cryptographic receipt validation and test-tampering results.
type VerificationStatus struct {
	Verified       bool            `json:"verified"`
	Reason         string          `json:"reason,omitempty"`
	CurrentDigest  string          `json:"currentDigest"`
	Receipt        *Receipt        `json:"receipt,omitempty"`
	TamperFindings []TamperFinding `json:"tamperFindings,omitempty"`
}

var skipDigestDirs = map[string]bool{
	".git": true, ".dtkit": true, "node_modules": true, "dist": true, "build": true,
	"target": true, "vendor": true, ".venv": true, "venv": true, "__pycache__": true, ".next": true,
}

// ComputeWorkspaceStateDigest hashes tracked/untracked workspace state (git HEAD + status/dirty contents when in git, or full tree walk otherwise).
func ComputeWorkspaceStateDigest(repoRoot string) (string, int, error) {
	if _, err := exec.LookPath("git"); err == nil {
		chk := exec.Command("git", "-C", repoRoot, "rev-parse", "--is-inside-work-tree")
		if out, gErr := chk.Output(); gErr == nil && strings.TrimSpace(string(out)) == "true" {
			return computeGitWorkspaceDigest(repoRoot)
		}
	}
	return computeWalkWorkspaceDigest(repoRoot)
}

func computeGitWorkspaceDigest(repoRoot string) (string, int, error) {
	h := sha256.New()
	if headOut, err := exec.Command("git", "-C", repoRoot, "rev-parse", "HEAD").Output(); err == nil {
		h.Write([]byte("HEAD:"))
		h.Write([]byte(strings.TrimSpace(string(headOut))))
		h.Write([]byte{'\n'})
	}
	statusOut, err := exec.Command("git", "-C", repoRoot, "status", "--porcelain=v1", "-z", "-uall").Output()
	if err != nil {
		return computeWalkWorkspaceDigest(repoRoot)
	}
	entries := strings.Split(string(statusOut), "\x00")
	var dirty []string
	for _, ent := range entries {
		if len(ent) < 4 {
			continue
		}
		rel := filepath.ToSlash(ent[3:])
		if strings.HasPrefix(rel, ".dtkit/local/") || rel == ".dtkit/local" {
			continue
		}
		dirty = append(dirty, ent[:2]+" "+rel)
	}
	sort.Strings(dirty)
	for _, entry := range dirty {
		h.Write([]byte(entry))
		h.Write([]byte{0})
		_, rel, _ := strings.Cut(entry, " ")
		if target, rErr := paths.ResolveTarget(repoRoot, rel); rErr == nil {
			if raw, readErr := os.ReadFile(target); readErr == nil {
				sum := sha256.Sum256(raw)
				h.Write(sum[:])
			} else {
				h.Write([]byte("missing"))
			}
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), len(dirty), nil
}

func computeWalkWorkspaceDigest(repoRoot string) (string, int, error) {
	var files []string
	err := filepath.WalkDir(repoRoot, func(fullPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		relRaw, rErr := filepath.Rel(repoRoot, fullPath)
		if rErr != nil {
			return nil
		}
		rel := filepath.ToSlash(relRaw)
		if rel == "." {
			return nil
		}
		info, lErr := os.Lstat(fullPath)
		if lErr != nil || paths.IsReparseOrSymlink(info) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if skipDigestDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode().IsRegular() {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, rel := range files {
		h.Write([]byte(rel))
		h.Write([]byte{0})
		raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err == nil {
			sum := sha256.Sum256(raw)
			h.Write(sum[:])
		}
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), len(files), nil
}

// RecordCheckPass writes verify-receipt.json for the current workspace digest and resets streak.
func RecordCheckPass(repoRoot string, passedChecks []string, now time.Time) error {
	digest, count, err := ComputeWorkspaceStateDigest(repoRoot)
	if err != nil {
		return err
	}
	rec := Receipt{
		SchemaVersion:   SchemaVersion1,
		WorkspaceDigest: digest,
		FileCount:       count,
		PassedChecks:    append([]string(nil), passedChecks...),
		VerifiedAt:      now.UTC().Format(time.RFC3339),
	}
	if err := writeLocalJSON(repoRoot, ReceiptRelPath, rec); err != nil {
		return err
	}
	return writeLocalJSON(repoRoot, StreakRelPath, Streak{
		SchemaVersion:       SchemaVersion1,
		ConsecutiveFailures: 0,
		UpdatedAt:           now.UTC().Format(time.RFC3339),
	})
}

// RecordCheckFailure increments consecutive check failures in .dtkit/local/check-streak.json.
func RecordCheckFailure(repoRoot string, failedChecks []string, now time.Time) (Streak, error) {
	cur, _ := LoadStreak(repoRoot)
	cur.SchemaVersion = SchemaVersion1
	cur.ConsecutiveFailures++
	cur.LastFailedChecks = append([]string(nil), failedChecks...)
	cur.UpdatedAt = now.UTC().Format(time.RFC3339)
	if err := writeLocalJSON(repoRoot, StreakRelPath, cur); err != nil {
		return cur, err
	}
	return cur, nil
}

// LoadStreak reads .dtkit/local/check-streak.json or returns zero streak when absent.
func LoadStreak(repoRoot string) (Streak, error) {
	target, err := paths.ResolveTarget(repoRoot, StreakRelPath)
	if err != nil {
		return Streak{}, err
	}
	raw, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return Streak{SchemaVersion: SchemaVersion1}, nil
	}
	if err != nil {
		return Streak{}, err
	}
	var s Streak
	if err := json.Unmarshal(raw, &s); err != nil {
		return Streak{SchemaVersion: SchemaVersion1}, nil
	}
	return s, nil
}

// VerifyWorkspace validates that no test tampering exists and the stored check receipt matches current code.
func VerifyWorkspace(repoRoot string) (VerificationStatus, error) {
	digest, _, err := ComputeWorkspaceStateDigest(repoRoot)
	if err != nil {
		return VerificationStatus{}, err
	}
	out := VerificationStatus{
		CurrentDigest: digest,
	}

	tamper, tErr := DetectTestTampering(repoRoot)
	if tErr == nil && len(tamper) > 0 {
		out.TamperFindings = tamper
	}

	target, err := paths.ResolveTarget(repoRoot, ReceiptRelPath)
	if err != nil {
		return out, err
	}
	raw, rErr := os.ReadFile(target)
	if errors.Is(rErr, os.ErrNotExist) {
		out.Reason = fmt.Sprintf("missing %s: run 'dtkit check' after your final edit", ReceiptRelPath)
		return out, nil
	}
	if rErr != nil {
		return out, rErr
	}
	var rec Receipt
	if err := json.Unmarshal(raw, &rec); err != nil {
		out.Reason = fmt.Sprintf("corrupt %s: re-run 'dtkit check'", ReceiptRelPath)
		return out, nil
	}
	out.Receipt = &rec
	if len(out.TamperFindings) > 0 {
		out.Reason = fmt.Sprintf("detected %d test tampering finding(s) in working tree", len(out.TamperFindings))
		return out, nil
	}
	if rec.WorkspaceDigest != digest {
		out.Reason = fmt.Sprintf("UNVERIFIED: workspace changed since last passing check (receipt=%s current=%s)", rec.WorkspaceDigest, digest)
		return out, nil
	}
	out.Verified = true
	return out, nil
}

func writeLocalJSON(repoRoot, relPath string, v any) error {
	target, err := paths.ResolveTarget(repoRoot, relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	if info, lErr := os.Lstat(target); lErr == nil && paths.IsReparseOrSymlink(info) {
		return apperr.WithPath(apperr.Conflict("symlink_target", fmt.Sprintf("refusing symlinked %s", relPath)), relPath)
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
