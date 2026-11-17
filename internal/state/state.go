// Package state validates installation metadata before it is used by a planner.
// It does not perform filesystem writes or authorize deletion.
package state

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/adapter"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/catalog"
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
	SchemaVersion     int                    `json:"schemaVersion"`
	ManagedBy         string                 `json:"managedBy"`
	Generation        uint64                 `json:"generation"`
	Scope             string                 `json:"scope"`
	Runtime           string                 `json:"runtime"`
	AdapterRevision   string                 `json:"adapterRevision"`
	CodexHome         string                 `json:"codexHome,omitempty"`
	RuntimeHome       string                 `json:"runtimeHome,omitempty"`
	Kit               Kit                    `json:"kit"`
	Artifacts         []Artifact             `json:"artifacts"`
	Hooks             []HookRegistration     `json:"hooks,omitempty"`
	LastTransactionID string                 `json:"lastTransactionID,omitempty"`
	Selection         *catalog.SelectionSpec `json:"selection,omitempty"`
	InstalledAt       string                 `json:"installedAt"`
	UpdatedAt         string                 `json:"updatedAt"`
}

// GlobalHome returns the bound runtime-specific global home path recorded in state.
func (s Installation) GlobalHome() string {
	if s.CodexHome != "" {
		return s.CodexHome
	}
	return s.RuntimeHome
}

// Registry tracks one or more installed kits in a single (runtime, scope).
// Single-kit registries serialize as v1/v2 Installation for backward compatibility;
// multi-kit registries serialize as schemaVersion 3.
type Registry struct {
	SchemaVersion     int                     `json:"schemaVersion"`
	ManagedBy         string                  `json:"managedBy"`
	Scope             string                  `json:"scope"`
	Runtime           string                  `json:"runtime"`
	AdapterRevision   string                  `json:"adapterRevision"`
	CodexHome         string                  `json:"codexHome,omitempty"`
	RuntimeHome       string                  `json:"runtimeHome,omitempty"`
	Kits              map[string]Installation `json:"kits"`
	Hooks             []HookRegistration      `json:"hooks,omitempty"`
	LastTransactionID string                  `json:"lastTransactionID,omitempty"`
	UpdatedAt         string                  `json:"updatedAt"`
}

func (r Registry) GlobalHome() string {
	if r.CodexHome != "" {
		return r.CodexHome
	}
	return r.RuntimeHome
}

// KitIDs returns sorted installed kit IDs in this registry.
func (r Registry) KitIDs() []string {
	ids := make([]string, 0, len(r.Kits))
	for id := range r.Kits {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// AllHooks returns active hook registrations for this (runtime, scope).
func (r Registry) AllHooks() []HookRegistration {
	if len(r.Hooks) > 0 {
		return append([]HookRegistration(nil), r.Hooks...)
	}
	for _, id := range r.KitIDs() {
		if len(r.Kits[id].Hooks) > 0 {
			return append([]HookRegistration(nil), r.Kits[id].Hooks...)
		}
	}
	return nil
}

// PeerOwnedWholeFiles returns a map of normalized Root+"\x00"+Path -> ownerKitID for
// whole-file artifacts owned by kits other than excludeKitID.
func (r Registry) PeerOwnedWholeFiles(excludeKitID string) map[string]string {
	out := map[string]string{}
	for _, kitID := range r.KitIDs() {
		if kitID == excludeKitID {
			continue
		}
		for _, a := range r.Kits[kitID].Artifacts {
			if a.Strategy == "whole-file" {
				out[strings.ToLower(a.Root+"\x00"+a.Path)] = kitID
			}
		}
	}
	return out
}

// ValidateSelf verifies structural, allowlist, and cross-kit disjoint ownership invariants of a v3 Registry.
func (r Registry) ValidateSelf() error {
	if r.SchemaVersion != 3 {
		return fmt.Errorf("unsupported installation registry schemaVersion %d (expected 3)", r.SchemaVersion)
	}
	if r.ManagedBy != "dtkit" || r.Runtime == "" {
		return fmt.Errorf("unsupported or invalid installation registry metadata")
	}
	profile, ok := adapter.Profile(r.Runtime)
	if !ok || r.AdapterRevision != profile.Revision {
		return fmt.Errorf("unsupported or invalid installation registry metadata")
	}
	if !profile.SupportsScope(r.Scope) {
		return fmt.Errorf("invalid installation registry scope")
	}
	if err := validateHomeBinding(profile, r.Scope, r.CodexHome, r.RuntimeHome); err != nil {
		return err
	}
	if r.UpdatedAt == "" {
		return fmt.Errorf("missing installation registry timestamp")
	}
	if len(r.Kits) < 2 {
		return fmt.Errorf("v3 multi-kit registry requires at least 2 installed kits")
	}
	if err := validateHookList(profile, r.Scope, "engineer", r.Hooks); err != nil {
		return err
	}
	wholeOwners := map[string]string{}
	for mapKey, inst := range r.Kits {
		if mapKey != inst.Kit.ID {
			return fmt.Errorf("registry kit key %q does not match installation kit.id %q", mapKey, inst.Kit.ID)
		}
		if err := inst.ValidateSelf(); err != nil {
			return fmt.Errorf("registry kit %q invalid: %w", mapKey, err)
		}
		if inst.Scope != r.Scope || inst.Runtime != r.Runtime || inst.AdapterRevision != r.AdapterRevision {
			return fmt.Errorf("registry kit %q metadata mismatches scope/runtime/revision", mapKey)
		}
		if inst.GlobalHome() != r.GlobalHome() {
			return fmt.Errorf("registry kit %q home binding mismatches registry", mapKey)
		}
		if len(inst.Hooks) > 0 {
			return fmt.Errorf("registry kit %q cannot carry per-kit hooks in v3 (use top-level hooks)", mapKey)
		}
		for _, a := range inst.Artifacts {
			if a.Strategy != "whole-file" {
				continue
			}
			k := strings.ToLower(a.Root + "\x00" + a.Path)
			if prev, exists := wholeOwners[k]; exists {
				return fmt.Errorf("whole-file artifact %s:%s claimed by multiple kits (%s and %s)", a.Root, a.Path, prev, mapKey)
			}
			wholeOwners[k] = mapKey
		}
	}
	return nil
}

// MarshalSnapshot serializes reg deterministically: 0 kits -> (nil, nil), 1 kit -> v1/v2 Installation,
// >= 2 kits -> v3 Registry.
func (r Registry) MarshalSnapshot(txID string) ([]byte, error) {
	if len(r.Kits) == 0 {
		return nil, nil
	}
	if len(r.Kits) == 1 {
		id := r.KitIDs()[0]
		inst := r.Kits[id]
		hooks := r.AllHooks()
		inst.Hooks = hooks
		if len(hooks) > 0 {
			inst.SchemaVersion = 2
		} else if inst.SchemaVersion == 0 || inst.SchemaVersion == 3 || inst.SchemaVersion == 2 {
			inst.SchemaVersion = 1
		}
		if txID != "" {
			inst.LastTransactionID = txID
		}
		return json.MarshalIndent(inst, "", "  ")
	}

	out := Registry{
		SchemaVersion:     3,
		ManagedBy:         "dtkit",
		Scope:             r.Scope,
		Runtime:           r.Runtime,
		AdapterRevision:   r.AdapterRevision,
		CodexHome:         r.CodexHome,
		RuntimeHome:       r.RuntimeHome,
		Kits:              make(map[string]Installation, len(r.Kits)),
		Hooks:             r.AllHooks(),
		LastTransactionID: txID,
		UpdatedAt:         r.UpdatedAt,
	}
	if out.LastTransactionID == "" {
		out.LastTransactionID = r.LastTransactionID
	}
	latestUpdated := out.UpdatedAt
	for _, id := range r.KitIDs() {
		inst := r.Kits[id]
		inst.Hooks = nil
		if inst.SchemaVersion == 0 || inst.SchemaVersion == 3 || inst.SchemaVersion == 2 {
			inst.SchemaVersion = 1
		}
		if latestUpdated == "" || inst.UpdatedAt > latestUpdated {
			latestUpdated = inst.UpdatedAt
		}
		out.Kits[id] = inst
	}
	out.UpdatedAt = latestUpdated
	if err := out.ValidateSelf(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(out, "", "  ")
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
	if err := validateHomeBinding(profile, s.Scope, s.CodexHome, s.RuntimeHome); err != nil {
		return err
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
	if s.Selection != nil {
		if s.Selection.Preset != "" && len(s.Selection.RequestedComponents) > 0 {
			return fmt.Errorf("installation selection cannot specify both preset and requestedComponents")
		}
		if len(s.Selection.ResolvedComponents) == 0 {
			return fmt.Errorf("installation selection missing resolvedComponents")
		}
	}
	return s.validateHooks(profile)
}

func validateHomeBinding(profile adapter.RuntimeProfile, scope, codexHome, runtimeHome string) error {
	if profile.UsesCodexHome {
		if (scope == "project" && (codexHome != "" || runtimeHome != "")) || (scope == "global" && (!filepath.IsAbs(codexHome) || runtimeHome != "")) {
			return fmt.Errorf("invalid Codex home binding")
		}
	} else if profile.GlobalHomeKey != "" {
		if (scope == "project" && (runtimeHome != "" || codexHome != "")) || (scope == "global" && (!filepath.IsAbs(runtimeHome) || codexHome != "")) {
			return fmt.Errorf("invalid %s home binding", profile.DisplayName)
		}
	} else if codexHome != "" || runtimeHome != "" {
		return fmt.Errorf("unexpected runtime home binding for %s", profile.DisplayName)
	}
	return nil
}

func (s Installation) validateHooks(profile adapter.RuntimeProfile) error {
	return validateHookList(profile, s.Scope, s.Kit.ID, s.Hooks)
}

func validateHookList(profile adapter.RuntimeProfile, scope, kitID string, hooks []HookRegistration) error {
	if !profile.HooksSupported && len(hooks) > 0 {
		return fmt.Errorf("hooks are not supported for runtime %q", profile.ID)
	}
	seenIDs := map[string]bool{}
	for _, h := range hooks {
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
		if !IsAllowedRuntimeArtifactPath(profile.ID, scope, kitID, h.Root, h.Path, "hooks-json") {
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
	case "gemini":
		return (handlerID == "check-changed" && event == "AfterTool") ||
			(handlerID == "guard-edit" && event == "BeforeTool")
	case "cursor":
		return handlerID == "check-changed" && event == "afterFileEdit"
	case "opencode":
		return handlerID == "check-changed" && event == "tool.execute.after"
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
