// Package state validates installation metadata before it is used by a planner.
// It does not perform filesystem writes or authorize deletion.
package state

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"dtkit.local/dtkit/internal/adapter/codex"
	"dtkit.local/dtkit/internal/paths"
)

// BlockState records only the bytes DTKit owns, not the whole shared file.
type BlockState struct {
	Digest      string `json:"digest"`
	Separator   string `json:"separator"`
	CreatedFile bool   `json:"createdFile"`
}

type Kit struct {
	ID         string `json:"id"`
	Version    string `json:"version"`
	Digest     string `json:"digest"`
	SourceKind string `json:"sourceKind,omitempty"`
}

type Artifact struct {
	Root     string      `json:"root"`
	Path     string      `json:"path"`
	Strategy string      `json:"strategy"`
	Digest   string      `json:"digest"`
	Block    *BlockState `json:"block,omitempty"`
}

type Installation struct {
	SchemaVersion     int        `json:"schemaVersion"`
	ManagedBy         string     `json:"managedBy"`
	Generation        uint64     `json:"generation"`
	Scope             string     `json:"scope"`
	Runtime           string     `json:"runtime"`
	AdapterRevision   string     `json:"adapterRevision"`
	CodexHome         string     `json:"codexHome,omitempty"`
	Kit               Kit        `json:"kit"`
	Artifacts         []Artifact `json:"artifacts"`
	LastTransactionID string     `json:"lastTransactionID,omitempty"`
	InstalledAt       string     `json:"installedAt"`
	UpdatedAt         string     `json:"updatedAt"`
}

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var kitIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// Validate checks metadata against the known output mapping of the adapter.
// Callers must also check on-disk content and roots immediately before writes.
func (s Installation) Validate(expected []codex.Artifact) error {
	if s.SchemaVersion != 1 || s.ManagedBy != "dtkit" || s.Generation == 0 || s.Runtime != "codex" || s.AdapterRevision != codex.AdapterRevision {
		return fmt.Errorf("unsupported or invalid installation metadata")
	}
	if s.Scope != "project" && s.Scope != "global" {
		return fmt.Errorf("invalid installation scope")
	}
	if (s.Scope == "project" && s.CodexHome != "") || (s.Scope == "global" && !filepath.IsAbs(s.CodexHome)) {
		return fmt.Errorf("invalid Codex home binding")
	}
	if !kitIDPattern.MatchString(s.Kit.ID) || len(s.Kit.ID) > 32 || !versionPattern.MatchString(s.Kit.Version) || !validDigest(s.Kit.Digest) {
		return fmt.Errorf("invalid installed kit")
	}
	if s.Kit.SourceKind != "" && s.Kit.SourceKind != "bundled" && s.Kit.SourceKind != "local" {
		return fmt.Errorf("invalid kit source kind")
	}
	if s.InstalledAt == "" || s.UpdatedAt == "" {
		return fmt.Errorf("missing installation timestamps")
	}
	if len(expected) == 0 || len(s.Artifacts) != len(expected) {
		return fmt.Errorf("installed artifact count does not match adapter")
	}
	known := make(map[string]codex.Artifact, len(expected))
	for _, a := range expected {
		if (s.Scope == "project" && a.Root != "project") || (s.Scope == "global" && a.Root != "user-home" && a.Root != "codex-home") {
			return fmt.Errorf("adapter artifact does not belong to scope")
		}
		key := strings.ToLower(a.Root + "\x00" + a.Path)
		if _, ok := known[key]; ok {
			return fmt.Errorf("duplicate adapter artifact")
		}
		known[key] = a
	}
	seen := make(map[string]bool, len(s.Artifacts))
	for _, a := range s.Artifacts {
		if !paths.ValidateRelativePath(a.Path) {
			return fmt.Errorf("unsafe artifact path %q", a.Path)
		}
		key := strings.ToLower(a.Root + "\x00" + a.Path)
		want, ok := known[key]
		if !ok || seen[key] || want.Root != a.Root || want.Path != a.Path || want.Strategy != a.Strategy {
			return fmt.Errorf("unknown, duplicate or mismatched artifact %q", a.Path)
		}
		seen[key] = true
		if !validDigest(a.Digest) {
			return fmt.Errorf("invalid digest for %q", a.Path)
		}
		if a.Strategy == "managed-block" {
			if a.Block == nil || a.Block.Digest != a.Digest || (a.Block.Separator != "" && a.Block.Separator != "\n" && a.Block.Separator != "\r\n") {
				return fmt.Errorf("invalid block metadata for %q", a.Path)
			}
		} else if a.Strategy != "whole-file" || a.Block != nil {
			return fmt.Errorf("invalid artifact strategy for %q", a.Path)
		}
	}
	return nil
}

// ValidateSelf checks structural & allowlist invariants of a state record without
// requiring the original catalog source (used by remove and recovery).
func (s Installation) ValidateSelf() error {
	if s.SchemaVersion != 1 || s.ManagedBy != "dtkit" || s.Generation == 0 || s.Runtime != "codex" || s.AdapterRevision != codex.AdapterRevision {
		return fmt.Errorf("unsupported or invalid installation metadata")
	}
	if s.Scope != "project" && s.Scope != "global" {
		return fmt.Errorf("invalid installation scope")
	}
	if (s.Scope == "project" && s.CodexHome != "") || (s.Scope == "global" && !filepath.IsAbs(s.CodexHome)) {
		return fmt.Errorf("invalid Codex home binding")
	}
	if !kitIDPattern.MatchString(s.Kit.ID) || len(s.Kit.ID) > 32 || !versionPattern.MatchString(s.Kit.Version) || !validDigest(s.Kit.Digest) {
		return fmt.Errorf("invalid installed kit")
	}
	if s.Kit.SourceKind != "" && s.Kit.SourceKind != "bundled" && s.Kit.SourceKind != "local" {
		return fmt.Errorf("invalid kit source kind")
	}
	if s.InstalledAt == "" || s.UpdatedAt == "" || len(s.Artifacts) == 0 {
		return fmt.Errorf("missing installation timestamps or artifacts")
	}
	seen := make(map[string]bool, len(s.Artifacts))
	for _, a := range s.Artifacts {
		if !IsAllowedArtifactPath(s.Scope, s.Kit.ID, a.Root, a.Path, a.Strategy) {
			return fmt.Errorf("artifact %q outside DTKit allowlist", a.Path)
		}
		key := strings.ToLower(a.Root + "\x00" + a.Path)
		if seen[key] {
			return fmt.Errorf("duplicate artifact %q", a.Path)
		}
		seen[key] = true
		if !validDigest(a.Digest) {
			return fmt.Errorf("invalid digest for %q", a.Path)
		}
		if a.Strategy == "managed-block" {
			if a.Block == nil || a.Block.Digest != a.Digest || (a.Block.Separator != "" && a.Block.Separator != "\n" && a.Block.Separator != "\r\n") {
				return fmt.Errorf("invalid block metadata for %q", a.Path)
			}
		} else if a.Strategy != "whole-file" || a.Block != nil {
			return fmt.Errorf("invalid artifact strategy for %q", a.Path)
		}
	}
	return nil
}

// IsAllowedArtifactPath enforces DTKit's path allowlist before any state-driven write or delete.
func IsAllowedArtifactPath(scope, kitID, root, relPath, strategy string) bool {
	if !paths.ValidateRelativePath(relPath) {
		return false
	}
	if scope == "project" && root != "project" {
		return false
	}
	if scope == "global" && root != "user-home" && root != "codex-home" {
		return false
	}
	switch strategy {
	case "managed-block":
		if relPath != "AGENTS.md" {
			return false
		}
		if scope == "global" && root != "codex-home" {
			return false
		}
		return true
	case "whole-file":
		if scope == "global" && root != "user-home" {
			return false
		}
		skillPrefix := ".agents/skills/dtkit-" + scope + "-"
		contentPrefix := ".dtkit/content/" + kitID + "/"
		if strings.HasPrefix(relPath, skillPrefix) {
			rest := strings.TrimPrefix(relPath, skillPrefix)
			parts := strings.Split(rest, "/")
			return len(parts) == 2 && parts[0] != "" && parts[1] == "SKILL.md"
		}
		return strings.HasPrefix(relPath, contentPrefix) && len(relPath) > len(contentPrefix)
	default:
		return false
	}
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}
