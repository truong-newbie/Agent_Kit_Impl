// Package state validates installation metadata before it is used by a planner.
// It does not perform filesystem writes or authorize deletion.
package state

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// BlockState records only the bytes DTKit owns, not the whole shared file.
type BlockState struct {
	Digest      string `json:"digest"`
	Separator   string `json:"separator"`
	CreatedFile bool   `json:"createdFile"`
}

type HookRegistration struct {
	ID             string `json:"id"`
	Event          string `json:"event"`
	HandlerID      string `json:"handlerId"`
	Root           string `json:"root"`
	Path           string `json:"path"`
	Command        string `json:"command"`
	CommandWindows string `json:"commandWindows"`
	Executable     string `json:"executable"`
	EntryDigest    string `json:"entryDigest"`
	TimeoutSec     int    `json:"timeoutSec"`
	CreatedFile    bool   `json:"createdFile"`
}

type Kit struct {
	ID            string `json:"id"`
	Version       string `json:"version"`
	Digest        string `json:"digest"`
	ArchiveSHA256 string `json:"archiveSha256,omitempty"`
	SourceKind    string `json:"sourceKind,omitempty"`
}

type Artifact struct {
	Root     string      `json:"root"`
	Path     string      `json:"path"`
	Strategy string      `json:"strategy"`
	Digest   string      `json:"digest"`
	Block    *BlockState `json:"block,omitempty"`
}

type Installation struct {
	SchemaVersion     int                `json:"schemaVersion"`
	ManagedBy         string             `json:"managedBy"`
	Generation        uint64             `json:"generation"`
	Scope             string             `json:"scope"`
	Runtime           string             `json:"runtime"`
	AdapterRevision   string             `json:"adapterRevision"`
	CodexHome         string             `json:"codexHome,omitempty"`
	RuntimeHome       string             `json:"runtimeHome,omitempty"`
	Kit               Kit                `json:"kit"`
	Artifacts         []Artifact         `json:"artifacts"`
	Hooks             []HookRegistration `json:"hooks,omitempty"`
	LastTransactionID string             `json:"lastTransactionID,omitempty"`
	InstalledAt       string             `json:"installedAt"`
	UpdatedAt         string             `json:"updatedAt"`
}

// GlobalHome returns the bound runtime-specific global home path recorded in state.
func (s Installation) GlobalHome() string {
	if s.CodexHome != "" {
		return s.CodexHome
	}
	return s.RuntimeHome
}

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
var kitIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

func (s Installation) validateCommon() error {
	if s.SchemaVersion != 1 && s.SchemaVersion != 2 {
		return fmt.Errorf("unsupported installation state schemaVersion %d (supported: 1, 2; upgrade dtkit or use a compatible version)", s.SchemaVersion)
	}
	if s.ManagedBy != "dtkit" || s.Generation == 0 || s.Runtime == "" {
		return fmt.Errorf("unsupported or invalid installation metadata")
	}
	profile, ok := adapter.Profile(s.Runtime)
	if !ok || s.AdapterRevision != profile.Revision {
		return fmt.Errorf("unsupported or invalid installation metadata")
	}
	if s.SchemaVersion == 1 && len(s.Hooks) > 0 {
		return fmt.Errorf("hooks require state schemaVersion 2")
	}
	if !profile.SupportsScope(s.Scope) {
		return fmt.Errorf("invalid installation scope")
	}
	if profile.UsesCodexHome {
		if (s.Scope == "project" && (s.CodexHome != "" || s.RuntimeHome != "")) || (s.Scope == "global" && (!filepath.IsAbs(s.CodexHome) || s.RuntimeHome != "")) {
			return fmt.Errorf("invalid Codex home binding")
		}
	} else if profile.GlobalHomeKey != "" {
		if (s.Scope == "project" && (s.RuntimeHome != "" || s.CodexHome != "")) || (s.Scope == "global" && (!filepath.IsAbs(s.RuntimeHome) || s.CodexHome != "")) {
			return fmt.Errorf("invalid %s home binding", profile.DisplayName)
		}
	} else if s.CodexHome != "" || s.RuntimeHome != "" {
		return fmt.Errorf("unexpected runtime home binding for %s", profile.DisplayName)
	}
	if !kitIDPattern.MatchString(s.Kit.ID) || len(s.Kit.ID) > 32 || !versionPattern.MatchString(s.Kit.Version) || !validDigest(s.Kit.Digest) {
		return fmt.Errorf("invalid installed kit")
	}
	if s.Kit.SourceKind != "" && s.Kit.SourceKind != "bundled" && s.Kit.SourceKind != "local" && s.Kit.SourceKind != "archive" && s.Kit.SourceKind != "remote" {
		return fmt.Errorf("invalid kit source kind")
	}
	if s.Kit.ArchiveSHA256 != "" && !validDigest(s.Kit.ArchiveSHA256) {
		return fmt.Errorf("invalid kit archiveSha256")
	}
	if s.InstalledAt == "" || s.UpdatedAt == "" {
		return fmt.Errorf("missing installation timestamps")
	}
	return s.validateHooks(profile)
}

func (s Installation) validateHooks(profile adapter.RuntimeProfile) error {
	if !profile.HooksSupported && len(s.Hooks) > 0 {
		return fmt.Errorf("hooks are not supported for runtime %q", s.Runtime)
	}
	seenIDs := map[string]bool{}
	for _, h := range s.Hooks {
		if !kitIDPattern.MatchString(h.ID) || !kitIDPattern.MatchString(h.HandlerID) {
			return fmt.Errorf("invalid hook id %q", h.ID)
		}
		if !validHookForRuntime(profile.ID, h.HandlerID, h.Event) {
			return fmt.Errorf("unsupported hook handler or event: %s/%s", h.HandlerID, h.Event)
		}
		if seenIDs[h.ID] {
			return fmt.Errorf("duplicate hook %q", h.ID)
		}
		seenIDs[h.ID] = true
		if !IsAllowedRuntimeArtifactPath(s.Runtime, s.Scope, s.Kit.ID, h.Root, h.Path, "hooks-json") {
			return fmt.Errorf("hook target %s:%s outside allowlist", h.Root, h.Path)
		}
		if strings.TrimSpace(h.Command) == "" || strings.TrimSpace(h.CommandWindows) == "" || !filepath.IsAbs(h.Executable) {
			return fmt.Errorf("invalid hook command or executable for %q", h.ID)
		}
		if !validDigest(h.EntryDigest) || h.TimeoutSec <= 0 || h.TimeoutSec > 30 {
			return fmt.Errorf("invalid hook digest or timeout for %q", h.ID)
		}
	}
	return nil
}

func validHookForRuntime(runtimeID, handlerID, event string) bool {
	switch runtimeID {
	case "codex":
		return handlerID == "session-context" && event == "SessionStart"
	case "claude":
		return (handlerID == "check-changed" && event == "PostToolUse") ||
			(handlerID == "guard-edit" && event == "PreToolUse")
	default:
		return false
	}
}

// Validate checks metadata against the known output mapping of the adapter.
// Callers must also check on-disk content and roots immediately before writes.
func (s Installation) Validate(expected []adapter.Artifact) error {
	if err := s.validateCommon(); err != nil {
		return err
	}
	profile, ok := adapter.Profile(s.Runtime)
	if !ok {
		return fmt.Errorf("unsupported runtime %q", s.Runtime)
	}
	if len(expected) == 0 || len(s.Artifacts) != len(expected) {
		return fmt.Errorf("installed artifact count does not match adapter")
	}
	known := make(map[string]adapter.Artifact, len(expected))
	for _, a := range expected {
		if !profile.AllowedRootForScope(s.Scope, a.Root) {
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
	if err := s.validateCommon(); err != nil {
		return err
	}
	if len(s.Artifacts) == 0 {
		return fmt.Errorf("missing installation artifacts")
	}
	seen := make(map[string]bool, len(s.Artifacts))
	for _, a := range s.Artifacts {
		if !IsAllowedRuntimeArtifactPath(s.Runtime, s.Scope, s.Kit.ID, a.Root, a.Path, a.Strategy) {
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

// IsAllowedArtifactPath enforces DTKit's Codex allowlist before any state-driven write or delete.
func IsAllowedArtifactPath(scope, kitID, root, relPath, strategy string) bool {
	return IsAllowedRuntimeArtifactPath(adapter.DefaultRuntime, scope, kitID, root, relPath, strategy)
}

// IsAllowedRuntimeArtifactPath enforces the named runtime's path allowlist before writes or deletes.
func IsAllowedRuntimeArtifactPath(runtime, scope, kitID, root, relPath, strategy string) bool {
	profile, ok := adapter.Profile(runtime)
	if !ok {
		return false
	}
	return profile.IsAllowedArtifactPath(scope, kitID, root, relPath, strategy)
}

func validDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}
