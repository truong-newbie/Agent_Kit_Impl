// Package adapter defines shared runtime profiles, artifact structures, and rendering primitives.
package adapter

import (
	"fmt"
	"path"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const DefaultRuntime = "codex"

// Artifact is a logical desired filesystem entry emitted by a runtime adapter.
type Artifact struct {
	Root        string
	Path        string
	Strategy    string
	ComponentID string
	Content     []byte
}

// CapabilityStatus distinguishes whether the target runtime supports a feature
// versus whether DTKit currently manages that feature for the runtime.
type CapabilityStatus struct {
	RuntimeSupported bool   `json:"runtimeSupported"`
	DTKitSupported   bool   `json:"dtkitSupported"`
	Note             string `json:"note,omitempty"`
}

// Capabilities describes runtime and DTKit support for a scope.
type Capabilities struct {
	Instructions CapabilityStatus `json:"instructions"`
	Skills       CapabilityStatus `json:"skills"`
	Hooks        CapabilityStatus `json:"hooks"`
}

// SkillLocation maps a scope to the root key and directory prefix holding rendered skills.
type SkillLocation struct {
	Root   string
	Dir    string
	Prefix string // directory & frontmatter skill name prefix ending in "-"
}

// InstructionLocation maps a scope to the root key and relative path of the instruction file.
type InstructionLocation struct {
	Root         string
	Path         string
	OverridePath string // optional file whose non-empty presence overrides Path
}

// RuntimeProfile captures the validated path layout, state paths, and capability matrix of a runtime.
type RuntimeProfile struct {
	ID              string
	DisplayName     string
	Revision        string
	Binary          string
	StateRelPath    string
	OperationalDir  string
	GlobalHomeKey   string // key in roots map for runtime-specific home (e.g. "codex-home", "gemini-home")
	GlobalHomeEnv   string
	UsesCodexHome   bool
	Scopes          []string
	HooksSupported  bool
	HookProjectPath string
	HookGlobalRoot  string
	HookGlobalPath  string
	Instructions    map[string]InstructionLocation
	Skills          map[string]SkillLocation
	ContentRoot     map[string]string // scope -> root key ("project" or "user-home")
	ContentRelBase  string            // e.g. ".dtkit/content" or ".dtkit/runtimes/gemini/content"
	CapabilitiesMap map[string]Capabilities
}

var profiles = map[string]RuntimeProfile{
	"codex": {
		ID:              "codex",
		DisplayName:     "Codex CLI",
		Revision:        "1",
		Binary:          "codex",
		StateRelPath:    ".dtkit/state.json",
		OperationalDir:  ".dtkit/local",
		GlobalHomeKey:   "codex-home",
		GlobalHomeEnv:   "CODEX_HOME",
		UsesCodexHome:   true,
		Scopes:          []string{"project", "global"},
		HooksSupported:  true,
		HookProjectPath: ".codex/hooks.json",
		HookGlobalRoot:  "codex-home",
		HookGlobalPath:  "hooks.json",
		Instructions: map[string]InstructionLocation{
			"project": {Root: "project", Path: "AGENTS.md", OverridePath: "AGENTS.override.md"},
			"global":  {Root: "codex-home", Path: "AGENTS.md", OverridePath: "AGENTS.override.md"},
		},
		Skills: map[string]SkillLocation{
			"project": {Root: "project", Dir: ".agents/skills", Prefix: "dtkit-project-"},
			"global":  {Root: "user-home", Dir: ".agents/skills", Prefix: "dtkit-global-"},
		},
		ContentRoot: map[string]string{
			"project": "project",
			"global":  "user-home",
		},
		ContentRelBase: ".dtkit/content",
		CapabilitiesMap: map[string]Capabilities{
			"project": {
				Instructions: CapabilityStatus{RuntimeSupported: true, DTKitSupported: true, Note: "AGENTS.md managed block"},
				Skills:       CapabilityStatus{RuntimeSupported: true, DTKitSupported: true, Note: ".agents/skills/dtkit-project-*"},
				Hooks:        CapabilityStatus{RuntimeSupported: true, DTKitSupported: true, Note: "opt-in .codex/hooks.json SessionStart"},
			},
			"global": {
				Instructions: CapabilityStatus{RuntimeSupported: true, DTKitSupported: true, Note: "$CODEX_HOME/AGENTS.md managed block"},
				Skills:       CapabilityStatus{RuntimeSupported: true, DTKitSupported: true, Note: "~/.agents/skills/dtkit-global-*"},
				Hooks:        CapabilityStatus{RuntimeSupported: true, DTKitSupported: true, Note: "opt-in $CODEX_HOME/hooks.json SessionStart"},
			},
		},
	},
	"gemini": {
		ID:             "gemini",
		DisplayName:    "Gemini CLI",
		Revision:       "1",
		Binary:         "gemini",
		StateRelPath:   ".dtkit/runtimes/gemini/state.json",
		OperationalDir: ".dtkit/local/runtimes/gemini",
		GlobalHomeKey:  "gemini-home",
		GlobalHomeEnv:  "GEMINI_CLI_HOME",
		Scopes:         []string{"project", "global"},
		HooksSupported: false,
		Instructions: map[string]InstructionLocation{
			"project": {Root: "project", Path: "GEMINI.md"},
			"global":  {Root: "gemini-home", Path: "GEMINI.md"},
		},
		Skills: map[string]SkillLocation{
			"project": {Root: "project", Dir: ".gemini/skills", Prefix: "dtkit-gemini-project-"},
			"global":  {Root: "gemini-home", Dir: "skills", Prefix: "dtkit-gemini-global-"},
		},
		ContentRoot: map[string]string{
			"project": "project",
			"global":  "user-home",
		},
		ContentRelBase: ".dtkit/runtimes/gemini/content",
		CapabilitiesMap: map[string]Capabilities{
			"project": {
				Instructions: CapabilityStatus{RuntimeSupported: true, DTKitSupported: true, Note: "GEMINI.md managed block"},
				Skills:       CapabilityStatus{RuntimeSupported: true, DTKitSupported: true, Note: ".gemini/skills/dtkit-gemini-project-*"},
				Hooks:        CapabilityStatus{RuntimeSupported: true, DTKitSupported: false, Note: "Gemini CLI supports hooks in .gemini/settings.json; not managed by DTKit"},
			},
			"global": {
				Instructions: CapabilityStatus{RuntimeSupported: true, DTKitSupported: true, Note: "~/.gemini/GEMINI.md managed block"},
				Skills:       CapabilityStatus{RuntimeSupported: true, DTKitSupported: true, Note: "~/.gemini/skills/dtkit-gemini-global-*"},
				Hooks:        CapabilityStatus{RuntimeSupported: true, DTKitSupported: false, Note: "Gemini CLI supports hooks in ~/.gemini/settings.json; not managed by DTKit"},
			},
		},
	},
}

// NormalizeRuntime returns DefaultRuntime when runtime is empty.
func NormalizeRuntime(runtime string) string {
	if runtime == "" {
		return DefaultRuntime
	}
	return runtime
}

// SupportedRuntimes returns implemented runtime identifiers in deterministic order.
func SupportedRuntimes() []string {
	return []string{"codex", "gemini"}
}

// Profile returns the RuntimeProfile for runtime if supported.
func Profile(runtime string) (RuntimeProfile, bool) {
	p, ok := profiles[NormalizeRuntime(runtime)]
	return p, ok
}

// MustProfile returns the RuntimeProfile for runtime or an error if unsupported.
func MustProfile(runtime string) (RuntimeProfile, error) {
	norm := NormalizeRuntime(runtime)
	p, ok := profiles[norm]
	if !ok {
		return RuntimeProfile{}, fmt.Errorf("unsupported runtime %q", runtime)
	}
	return p, nil
}

// SupportsScope reports whether the runtime profile supports scope.
func (p RuntimeProfile) SupportsScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// ScopeStateRoot returns the root key holding DTKit state and operational files for scope.
func ScopeStateRoot(scope string) string {
	if scope == "global" {
		return "user-home"
	}
	return "project"
}

// GlobalHomeRoot returns the runtime's configured global home binding path from roots.
func (p RuntimeProfile) GlobalHomeRoot(roots map[string]string) string {
	return roots[p.GlobalHomeKey]
}

// AllowedRootForScope checks whether root is valid for the runtime in scope.
func (p RuntimeProfile) AllowedRootForScope(scope, root string) bool {
	if scope == "project" {
		return root == "project"
	}
	if scope != "global" {
		return false
	}
	return root == "user-home" || (p.GlobalHomeKey != "" && root == p.GlobalHomeKey)
}

// SkillPathPrefix returns "<dir>/<prefix>" (e.g. ".agents/skills/dtkit-project-").
func (p RuntimeProfile) SkillPathPrefix(scope string) string {
	loc, ok := p.Skills[scope]
	if !ok {
		return ""
	}
	if loc.Dir == "" {
		return loc.Prefix
	}
	return loc.Dir + "/" + loc.Prefix
}

// IsSkillArtifact reports whether relPath is a DTKit-managed SKILL.md for this runtime and scope.
func (p RuntimeProfile) IsSkillArtifact(scope, relPath string) bool {
	prefix := p.SkillPathPrefix(scope)
	if prefix == "" || !strings.HasPrefix(relPath, prefix) {
		return false
	}
	rest := strings.TrimPrefix(relPath, prefix)
	parts := strings.Split(rest, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] == "SKILL.md"
}

// IsAllowedArtifactPath validates that a state/desired artifact stays strictly within this runtime's allowlist.
func (p RuntimeProfile) IsAllowedArtifactPath(scope, kitID, root, relPath, strategy string) bool {
	if !paths.ValidateRelativePath(relPath) || !p.AllowedRootForScope(scope, root) {
		return false
	}
	switch strategy {
	case "managed-block":
		loc, ok := p.Instructions[scope]
		return ok && root == loc.Root && relPath == loc.Path
	case "hooks-json":
		if !p.HooksSupported {
			return false
		}
		if scope == "project" {
			return root == "project" && relPath == p.HookProjectPath
		}
		return root == p.HookGlobalRoot && relPath == p.HookGlobalPath
	case "whole-file":
		skillLoc, hasSkills := p.Skills[scope]
		if hasSkills && root == skillLoc.Root && p.IsSkillArtifact(scope, relPath) {
			return true
		}
		contentRoot, hasContent := p.ContentRoot[scope]
		if !hasContent || root != contentRoot {
			return false
		}
		contentPrefix := p.ContentRelBase + "/" + kitID + "/"
		return strings.HasPrefix(relPath, contentPrefix) && len(relPath) > len(contentPrefix)
	default:
		return false
	}
}

// OverrideCheckPath returns the sibling override filename that blocks managed-block writes (if any).
func (p RuntimeProfile) OverrideCheckPath(scope, relPath string) string {
	loc, ok := p.Instructions[scope]
	if !ok || relPath != loc.Path {
		return ""
	}
	return loc.OverridePath
}

// StateDir returns the parent directory containing StateRelPath.
func (p RuntimeProfile) StateDir() string {
	return path.Dir(p.StateRelPath)
}

// IsAllowedCreatedDir reports whether DTKit is allowed to track and prune an empty created directory.
func (p RuntimeProfile) IsAllowedCreatedDir(scope, kitID, root, rel string) bool {
	if !paths.ValidateRelativePath(rel) || !p.AllowedRootForScope(scope, root) {
		return false
	}
	stateRoot := ScopeStateRoot(scope)
	if root == stateRoot {
		if isDirAncestorOf(rel, p.StateDir()) {
			return true
		}
	}
	if contentRoot, ok := p.ContentRoot[scope]; ok && root == contentRoot {
		kitContentBase := p.ContentRelBase + "/" + kitID
		if isDirAncestorOf(rel, kitContentBase) || strings.HasPrefix(rel, kitContentBase+"/") {
			return true
		}
	}
	if skillLoc, ok := p.Skills[scope]; ok && root == skillLoc.Root {
		if skillLoc.Dir != "" && isDirAncestorOf(rel, skillLoc.Dir) {
			return true
		}
		if strings.HasPrefix(rel, p.SkillPathPrefix(scope)) {
			return true
		}
	}
	return false
}

func isDirAncestorOf(candidate, target string) bool {
	if candidate == "" || target == "" {
		return false
	}
	return candidate == target || strings.HasPrefix(target, candidate+"/")
}
