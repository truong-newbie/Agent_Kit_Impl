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
	"dtkit.local/dtkit/internal/planner"
)

type Kit struct {
	ID string `json:"id"`
	Version string `json:"version"`
	Digest string `json:"digest"`
}

type Artifact struct {
	Root string `json:"root"`
	Path string `json:"path"`
	Strategy string `json:"strategy"`
	Digest string `json:"digest"`
	Block *planner.BlockState `json:"block,omitempty"`
}

type Installation struct {
	SchemaVersion int `json:"schemaVersion"`
	ManagedBy string `json:"managedBy"`
	Generation uint64 `json:"generation"`
	Scope string `json:"scope"`
	Runtime string `json:"runtime"`
	AdapterRevision string `json:"adapterRevision"`
	CodexHome string `json:"codexHome,omitempty"`
	Kit Kit `json:"kit"`
	Artifacts []Artifact `json:"artifacts"`
}

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// Validate checks metadata against the known output mapping of the adapter.
// Callers must also check on-disk content and roots immediately before writes.
func (s Installation) Validate(expected []codex.Artifact) error {
	if s.SchemaVersion != 1 || s.ManagedBy != "dtkit" || s.Generation == 0 || s.Runtime != "codex" || s.AdapterRevision != "1" { return fmt.Errorf("unsupported or invalid installation metadata") }
	if s.Scope != "project" && s.Scope != "global" { return fmt.Errorf("invalid installation scope") }
	if (s.Scope == "project" && s.CodexHome != "") || (s.Scope == "global" && !filepath.IsAbs(s.CodexHome)) { return fmt.Errorf("invalid Codex home binding") }
	if s.Kit.ID != "engineer" || !versionPattern.MatchString(s.Kit.Version) || !validDigest(s.Kit.Digest) { return fmt.Errorf("invalid installed kit") }
	if len(expected) == 0 || len(s.Artifacts) != len(expected) { return fmt.Errorf("installed artifact count does not match adapter") }
	known := make(map[string]codex.Artifact, len(expected))
	for _, a := range expected {
		if (s.Scope == "project" && a.Root != "project") || (s.Scope == "global" && a.Root != "user-home" && a.Root != "codex-home") { return fmt.Errorf("adapter artifact does not belong to scope") }
		key := strings.ToLower(a.Root + "\x00" + a.Path)
		if _, ok := known[key]; ok { return fmt.Errorf("duplicate adapter artifact") }
		known[key] = a
	}
	seen := make(map[string]bool, len(s.Artifacts))
	for _, a := range s.Artifacts {
		key := strings.ToLower(a.Root + "\x00" + a.Path)
		want, ok := known[key]
		if !ok || seen[key] || want.Root != a.Root || want.Path != a.Path || want.Strategy != a.Strategy { return fmt.Errorf("unknown, duplicate or mismatched artifact %q", a.Path) }
		seen[key] = true
		if !validDigest(a.Digest) { return fmt.Errorf("invalid digest for %q", a.Path) }
		if a.Strategy == "managed-block" {
			if a.Block == nil || a.Block.Digest != a.Digest || (a.Block.Separator != "" && a.Block.Separator != "\n" && a.Block.Separator != "\r\n") { return fmt.Errorf("invalid block metadata for %q", a.Path) }
		} else if a.Strategy != "whole-file" || a.Block != nil { return fmt.Errorf("invalid artifact strategy for %q", a.Path) }
	}
	return nil
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}
