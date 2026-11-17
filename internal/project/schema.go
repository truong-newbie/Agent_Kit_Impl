// Package project provides read-only repository inspection and strict .dtkit/project.json management.
package project

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

const (
	SchemaVersion1    = 1
	ProfileRelPath    = ".dtkit/project.json"
	DefaultTimeoutSec = 120
	MaxTimeoutSec     = 3600
	MaxProfileBytes   = 1 << 20
)

var (
	checkIDPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	envNamePattern  = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	validStacks     = map[string]bool{"go": true, "node": true, "ts": true, "python": true, "rust": true, "generic": true}
	validCheckKinds = map[string]bool{"test": true, "lint": true, "typecheck": true, "format": true, "build": true, "custom": true}
	blockedEnvNames = map[string]bool{
		"PATH": true, "LD_PRELOAD": true, "LD_LIBRARY_PATH": true, "DYLD_INSERT_LIBRARIES": true,
		"HOME": true, "USERPROFILE": true, "ANTHROPIC_API_KEY": true, "OPENAI_API_KEY": true,
		"GITHUB_TOKEN": true, "GH_TOKEN": true, "AWS_SECRET_ACCESS_KEY": true,
	}
)

// Profile represents .dtkit/project.json (schemaVersion 1).
type Profile struct {
	SchemaVersion int             `json:"schemaVersion"`
	Name          string          `json:"name,omitempty"`
	Roots         []WorkspaceRoot `json:"roots"`
	Checks        []CheckSpec     `json:"checks"`
	Include       []string        `json:"include,omitempty"`
	Exclude       []string        `json:"exclude,omitempty"`
	GeneratedDirs []string        `json:"generatedDirs,omitempty"`
	Evidence      []EvidenceItem  `json:"evidence,omitempty"`
}

// WorkspaceRoot describes one detected code root within the repository.
type WorkspaceRoot struct {
	Path           string   `json:"path"`
	Stack          string   `json:"stack"`
	PackageManager string   `json:"packageManager,omitempty"`
	Evidence       []string `json:"evidence,omitempty"`
}

// CheckSpec describes a single verification command executed strictly via argv (no shell wrapping).
type CheckSpec struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Executable   string   `json:"executable"`
	Argv         []string `json:"argv"`
	Cwd          string   `json:"cwd"`
	TimeoutSec   int      `json:"timeoutSec"`
	AllowEnv     []string `json:"allowEnv,omitempty"`
	MutatesFiles bool     `json:"mutatesFiles,omitempty"`
}

// EvidenceItem records a read-only discovery source for transparency.
type EvidenceItem struct {
	Source  string `json:"source"`
	Summary string `json:"summary"`
}

// ParseAndValidate decodes raw JSON bytes strictly against Project Profile v1 rules.
func ParseAndValidate(data []byte) (Profile, error) {
	if len(data) == 0 {
		return Profile{}, apperr.Usage("invalid_project_profile", "project profile is empty")
	}
	if len(data) > MaxProfileBytes {
		return Profile{}, apperr.Usage("invalid_project_profile", "project profile exceeds size limit")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var p Profile
	if err := dec.Decode(&p); err != nil {
		return Profile{}, apperr.Usage("invalid_project_profile", fmt.Sprintf("invalid project profile JSON: %v", err))
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Profile{}, apperr.Usage("invalid_project_profile", "unexpected trailing JSON data in project profile")
	}
	if err := ValidateProfile(p); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// ValidateProfile verifies structural, path-safety, and argv command safety invariants.
func ValidateProfile(p Profile) error {
	if p.SchemaVersion != SchemaVersion1 {
		return apperr.Usage("invalid_project_profile", fmt.Sprintf("unsupported project schemaVersion %d (expected %d)", p.SchemaVersion, SchemaVersion1))
	}
	if p.Roots == nil || p.Checks == nil {
		return apperr.Usage("invalid_project_profile", "roots and checks arrays are required")
	}
	if len(p.Name) > 128 {
		return apperr.Usage("invalid_project_profile", "project name exceeds 128 characters")
	}

	seenRoots := map[string]bool{}
	for _, r := range p.Roots {
		normPath, err := validateWorkspaceRel(r.Path, true)
		if err != nil {
			return apperr.Usage("invalid_project_profile", fmt.Sprintf("invalid root path %q: %v", r.Path, err))
		}
		if !validStacks[r.Stack] {
			return apperr.Usage("invalid_project_profile", fmt.Sprintf("unsupported stack %q for root %q", r.Stack, r.Path))
		}
		key := normPath + ":" + r.Stack
		if seenRoots[key] {
			return apperr.Usage("invalid_project_profile", fmt.Sprintf("duplicate root/stack entry %q", key))
		}
		seenRoots[key] = true
		for _, ev := range r.Evidence {
			if _, err := validateWorkspaceRel(ev, false); err != nil {
				return apperr.Usage("invalid_project_profile", fmt.Sprintf("invalid root evidence path %q: %v", ev, err))
			}
		}
	}

	seenChecks := map[string]bool{}
	for _, c := range p.Checks {
		if !checkIDPattern.MatchString(c.ID) {
			return apperr.Usage("invalid_project_profile", fmt.Sprintf("invalid check id %q", c.ID))
		}
		if seenChecks[c.ID] {
			return apperr.Usage("invalid_project_profile", fmt.Sprintf("duplicate check id %q", c.ID))
		}
		seenChecks[c.ID] = true
		if !validCheckKinds[c.Kind] {
			return apperr.Usage("invalid_project_profile", fmt.Sprintf("unsupported check kind %q for %q", c.Kind, c.ID))
		}
		if err := validateExecutableAndArgv(c.Executable, c.Argv); err != nil {
			return apperr.Usage("invalid_project_profile", fmt.Sprintf("unsafe check %q: %v", c.ID, err))
		}
		if _, err := validateWorkspaceRel(c.Cwd, true); err != nil {
			return apperr.Usage("invalid_project_profile", fmt.Sprintf("invalid cwd %q for check %q: %v", c.Cwd, c.ID, err))
		}
		if c.TimeoutSec < 1 || c.TimeoutSec > MaxTimeoutSec {
			return apperr.Usage("invalid_project_profile", fmt.Sprintf("timeoutSec for %q must be between 1 and %d", c.ID, MaxTimeoutSec))
		}
		for _, envName := range c.AllowEnv {
			if !envNamePattern.MatchString(envName) || blockedEnvNames[envName] || strings.Contains(envName, "SECRET") || strings.Contains(envName, "TOKEN") || strings.Contains(envName, "KEY") || strings.Contains(envName, "PASS") {
				return apperr.Usage("invalid_project_profile", fmt.Sprintf("disallowed allowEnv name %q in check %q", envName, c.ID))
			}
		}
	}

	for _, list := range [][]string{p.Include, p.Exclude, p.GeneratedDirs} {
		for _, item := range list {
			if _, err := validateWorkspaceRel(item, false); err != nil {
				return apperr.Usage("invalid_project_profile", fmt.Sprintf("invalid relative pattern/dir %q: %v", item, err))
			}
		}
	}
	for _, ev := range p.Evidence {
		if strings.TrimSpace(ev.Source) == "" || strings.TrimSpace(ev.Summary) == "" {
			return apperr.Usage("invalid_project_profile", "evidence entries require non-empty source and summary")
		}
	}
	return nil
}

func validateWorkspaceRel(rel string, allowDot bool) (string, error) {
	if rel == "" {
		return "", errors.New("path cannot be empty")
	}
	if strings.ContainsRune(rel, '\x00') || strings.Contains(rel, "\n") || strings.Contains(rel, "\r") {
		return "", errors.New("path contains control characters")
	}
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) || filepath.IsAbs(rel) {
		return "", errors.New("path must be relative to workspace")
	}
	if len(rel) >= 2 && ((rel[0] >= 'A' && rel[0] <= 'Z') || (rel[0] >= 'a' && rel[0] <= 'z')) && rel[1] == ':' {
		return "", errors.New("Windows drive paths are not allowed")
	}
	norm := strings.ReplaceAll(rel, `\`, "/")
	if allowDot && norm == "." {
		return ".", nil
	}
	cleaned := path.Clean(norm)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", errors.New("path traversal is not allowed")
	}
	if !allowDot {
		if !paths.ValidateRelativePath(cleaned) {
			return "", errors.New("invalid relative workspace path")
		}
	}
	return cleaned, nil
}

func validateExecutableAndArgv(exe string, argv []string) error {
	if strings.TrimSpace(exe) == "" || exe != strings.TrimSpace(exe) {
		return errors.New("executable must be non-empty without surrounding whitespace")
	}
	if strings.ContainsAny(exe, "\r\n\x00;|&`$<>") {
		return errors.New("executable contains forbidden shell metacharacters")
	}
	if filepath.IsAbs(exe) || strings.Contains(exe, "..") || strings.HasPrefix(exe, "~") {
		return errors.New("executable must be a command name on PATH (not an absolute or traversal path)")
	}
	if argv == nil {
		return errors.New("argv must be an array")
	}
	base := strings.ToLower(strings.TrimSuffix(path.Base(strings.ReplaceAll(exe, `\`, "/")), ".exe"))
	switch base {
	case "sh", "bash", "zsh", "dash", "fish", "pwsh", "powershell", "cmd":
		return fmt.Errorf("shell interpreter %q is not allowed directly in project checks", exe)
	}
	for _, a := range argv {
		if strings.ContainsRune(a, '\x00') {
			return errors.New("argv entry contains NUL byte")
		}
	}
	return nil
}

// CommandsTrustDigest computes a canonical SHA-256 over executable checks for explicit trust tracking.
func CommandsTrustDigest(p Profile) string {
	checks := make([]CheckSpec, len(p.Checks))
	copy(checks, p.Checks)
	sort.Slice(checks, func(i, j int) bool {
		return checks[i].ID < checks[j].ID
	})
	raw, _ := json.Marshal(checks)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
