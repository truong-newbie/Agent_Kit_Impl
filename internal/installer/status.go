package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter/codex"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/planner"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/state"
)

const (
	maxStateBytes    = state.MaxStateBytes
	maxArtifactBytes = 8 * 1024 * 1024
)

type ArtifactStatus struct {
	Root   string `json:"root"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type StatusReport struct {
	Installed           bool              `json:"installed"`
	Healthy             bool              `json:"healthy"`
	Classification      string            `json:"classification"` // not_installed | healthy | modified | missing | conflicted | recovery_required | profile_mismatch
	ProfileMismatch     bool              `json:"profileMismatch,omitempty"`
	RecoveryNeeded      bool              `json:"recoveryNeeded,omitempty"`
	PendingTransactions []PendingTx       `json:"pendingTransactions,omitempty"`
	UnknownFiles        []string          `json:"unknownFiles,omitempty"`
	Scope               string            `json:"scope"`
	Roots               map[string]string `json:"roots,omitempty"`
	KitID               string            `json:"kitId,omitempty"`
	KitVersion          string            `json:"kitVersion,omitempty"`
	KitDigest           string            `json:"kitDigest,omitempty"`
	BundledVersion      string            `json:"bundledVersion,omitempty"`
	AdapterRevision     string            `json:"adapterRevision,omitempty"`
	Artifacts           []ArtifactStatus  `json:"artifacts,omitempty"`
	Detail              string            `json:"detail,omitempty"`
}

// Inspect reads DTKit state and compares its owned bytes with disk. It never
// repairs, removes, or creates anything.
func Inspect(scope string, roots map[string]string, _ []codex.Artifact) (StatusReport, error) {
	report := StatusReport{Scope: scope, Roots: roots, Classification: "not_installed"}
	pending, lockStatus, err := InspectTransactions(scope, roots)
	if err != nil {
		return report, err
	}
	if len(pending) > 0 || lockStatus.Present {
		report.RecoveryNeeded = true
		report.Classification = "recovery_required"
		report.PendingTransactions = pending
		report.Detail = "unfinished transaction or lock detected; run dtkit recover"
	}

	statePath, err := planner.ResolveTarget(roots[scopeRoot(scope)], ".dtkit/state.json")
	if err != nil {
		return report, err
	}
	installed, exists, err := state.LoadFile(statePath, scope)
	if err != nil {
		return report, err
	}
	if !exists {
		if !report.RecoveryNeeded {
			report.Detail = "no DTKit state found"
		}
		return report, nil
	}

	report.Installed = true
	report.Healthy = !report.RecoveryNeeded
	report.KitID = installed.Kit.ID
	report.KitVersion = installed.Kit.Version
	report.KitDigest = installed.Kit.Digest
	report.AdapterRevision = installed.AdapterRevision

	if scope == "global" && !samePath(installed.CodexHome, roots["codex-home"]) {
		report.Healthy = false
		report.ProfileMismatch = true
		report.Classification = "profile_mismatch"
		report.Detail = fmt.Sprintf("installed Codex home %q does not match current %q", installed.CodexHome, roots["codex-home"])
		return report, nil
	}

	hasMissing := false
	hasModified := false
	hasConflict := false
	seenSkillDirs := map[string]bool{}

	for _, artifact := range installed.Artifacts {
		target, err := planner.ResolveTarget(roots[artifact.Root], artifact.Path)
		if err != nil {
			return report, err
		}
		item := ArtifactStatus{Root: artifact.Root, Path: artifact.Path, Status: "healthy"}
		tInfo, err := os.Lstat(target)
		if os.IsNotExist(err) {
			item.Status, item.Detail, report.Healthy = "missing", "owned file is missing", false
			hasMissing = true
			report.Artifacts = append(report.Artifacts, item)
			continue
		}
		if err != nil {
			return report, err
		}
		if paths.IsReparseOrSymlink(tInfo) || !tInfo.Mode().IsRegular() {
			item.Status, item.Detail, report.Healthy = "conflicted", "target is not a regular file", false
			hasConflict = true
			report.Artifacts = append(report.Artifacts, item)
			continue
		}
		if tInfo.Size() > maxArtifactBytes {
			item.Status, item.Detail, report.Healthy = "conflicted", "target exceeds 8 MiB size limit", false
			hasConflict = true
			report.Artifacts = append(report.Artifacts, item)
			continue
		}
		content, err := readRegularLimited(target, maxArtifactBytes)
		if err != nil {
			return report, err
		}
		if artifact.Strategy == "managed-block" {
			actual, bErr := planner.BlockDigestForKit(installed.Kit.ID, content, scope)
			if bErr != nil {
				item.Status, item.Detail, report.Healthy = "conflicted", bErr.Error(), false
				hasConflict = true
			} else if actual != artifact.Digest {
				item.Status, item.Detail, report.Healthy = "modified", "managed block differs from recorded state", false
				hasModified = true
			}
		} else {
			if sha256Hex(content) != artifact.Digest {
				item.Status, item.Detail, report.Healthy = "modified", "owned content differs from recorded state", false
				hasModified = true
			}
			if strings.HasPrefix(artifact.Path, ".agents/skills/dtkit-"+scope+"-") {
				skillDirRel := path.Dir(artifact.Path)
				if !seenSkillDirs[skillDirRel] {
					seenSkillDirs[skillDirRel] = true
					collectUnknownSkillFiles(roots[artifact.Root], skillDirRel, &report.UnknownFiles)
				}
			}
		}
		report.Artifacts = append(report.Artifacts, item)
	}

	sort.Strings(report.UnknownFiles)
	if !report.RecoveryNeeded {
		switch {
		case hasConflict:
			report.Classification = "conflicted"
		case hasMissing:
			report.Classification = "missing"
		case hasModified:
			report.Classification = "modified"
		default:
			report.Classification = "healthy"
		}
	}
	return report, nil
}

func collectUnknownSkillFiles(rootAbs, skillDirRel string, out *[]string) {
	dirAbs, err := planner.ResolveTarget(rootAbs, skillDirRel)
	if err != nil {
		return
	}
	entries, err := os.ReadDir(dirAbs)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Name() == "SKILL.md" {
			continue
		}
		*out = append(*out, path.Join(skillDirRel, entry.Name()))
	}
}

func readRegularLimited(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, apperr.WithPath(apperr.Conflict("file_too_large", fmt.Sprintf("file exceeds %d bytes", maxBytes)), path)
	}
	return data, nil
}

func samePath(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}

func scopeRoot(scope string) string {
	if scope == "global" {
		return "user-home"
	}
	return "project"
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
