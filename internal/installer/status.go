package installer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/planner"
	"dtkit.local/dtkit/internal/state"
)

const maxStateBytes = 1024 * 1024

type ArtifactStatus struct {
	Root   string `json:"root"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type StatusReport struct {
	Installed bool             `json:"installed"`
	Healthy   bool             `json:"healthy"`
	Scope     string           `json:"scope"`
	Artifacts []ArtifactStatus `json:"artifacts,omitempty"`
	Detail    string           `json:"detail,omitempty"`
}

// Inspect reads DTKit state and compares its owned bytes with disk. It never
// repairs, removes, or creates anything.
func Inspect(scope string, roots map[string]string, expected []codex.Artifact) (StatusReport, error) {
	report := StatusReport{Scope: scope}
	statePath, err := planner.ResolveTarget(roots[scopeRoot(scope)], ".dtkit/state.json")
	if err != nil {
		return report, err
	}
	file, err := os.Open(statePath)
	if os.IsNotExist(err) {
		report.Detail = "no DTKit state found"
		return report, nil
	}
	if err != nil {
		return report, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxStateBytes+1))
	if err != nil {
		return report, err
	}
	if len(data) > maxStateBytes {
		return report, fmt.Errorf("installation state is too large")
	}
	var installed state.Installation
	if err := json.Unmarshal(data, &installed); err != nil {
		return report, fmt.Errorf("invalid installation state: %w", err)
	}
	if installed.Scope != scope {
		return report, fmt.Errorf("installation state scope does not match request")
	}
	if err := installed.Validate(expected); err != nil {
		return report, err
	}
	report.Installed = true
	report.Healthy = true
	for _, artifact := range installed.Artifacts {
		target, err := planner.ResolveTarget(roots[artifact.Root], artifact.Path)
		if err != nil {
			return report, err
		}
		content, err := os.ReadFile(target)
		item := ArtifactStatus{Root: artifact.Root, Path: artifact.Path, Status: "healthy"}
		if os.IsNotExist(err) {
			item.Status, item.Detail, report.Healthy = "missing", "owned file is missing", false
		} else if err != nil {
			return report, err
		} else {
			actual := ""
			if artifact.Strategy == "managed-block" {
				actual, err = planner.BlockDigest(content, scope)
			} else {
				actual = sha256Hex(content)
			}
			if err != nil || actual != artifact.Digest {
				item.Status, item.Detail, report.Healthy = "modified", "owned content differs from recorded state", false
			}
		}
		report.Artifacts = append(report.Artifacts, item)
	}
	return report, nil
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
