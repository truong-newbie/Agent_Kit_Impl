// Package paths resolves project/global roots and enforces path containment.
package paths

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

var reservedDevicePattern = regexp.MustCompile(`^(COM|LPT)[1-9]$`)

// Environment abstracts OS/process lookups for deterministic testing.
type Environment struct {
	Getwd        func() (string, error)
	UserHomeDir  func() (string, error)
	UserCacheDir func() (string, error)
	Getenv       func(string) string
	GitTopLevel  func(ctx context.Context, dir string) (string, error)
}

func DefaultEnvironment() Environment {
	return Environment{
		Getwd:        os.Getwd,
		UserHomeDir:  os.UserHomeDir,
		UserCacheDir: os.UserCacheDir,
		Getenv:       os.Getenv,
		GitTopLevel:  gitTopLevel,
	}
}

func (e Environment) withDefaults() Environment {
	d := DefaultEnvironment()
	if e.Getwd == nil {
		e.Getwd = d.Getwd
	}
	if e.UserHomeDir == nil {
		e.UserHomeDir = d.UserHomeDir
	}
	if e.UserCacheDir == nil {
		e.UserCacheDir = d.UserCacheDir
	}
	if e.Getenv == nil {
		e.Getenv = d.Getenv
	}
	if e.GitTopLevel == nil {
		e.GitTopLevel = d.GitTopLevel
	}
	return e
}

func gitTopLevel(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ProjectResolution contains the resolved project root and diagnostics.
type ProjectResolution struct {
	Root     string
	Warnings []string
}

// GlobalResolution contains the resolved global roots and diagnostics.
type GlobalResolution struct {
	UserHome     string
	CodexHome    string
	GeminiHome   string
	OpenCodeHome string
	ClaudeHome   string
	Warnings     []string
}

// ResolveProject resolves the project root using --path, Git root, or CWD.
// It rejects filesystem roots, user home directories, and symlink roots.
func ResolveProject(ctx context.Context, env Environment, explicitPath string) (ProjectResolution, error) {
	env = env.withDefaults()
	var res ProjectResolution
	candidate := explicitPath
	if candidate == "" {
		cwd, err := env.Getwd()
		if err != nil {
			return res, err
		}
		candidate = cwd
		if top, err := env.GitTopLevel(ctx, cwd); err == nil && top != "" {
			candidate = top
		} else {
			res.Warnings = append(res.Warnings, "not inside a Git repository; using current working directory")
		}
	} else {
		cwd, err := env.Getwd()
		if err == nil {
			if top, gitErr := env.GitTopLevel(ctx, cwd); gitErr == nil && top != "" {
				absCandidate, absErr := filepath.Abs(candidate)
				absTop, topErr := filepath.Abs(top)
				if absErr == nil && topErr == nil && !strings.EqualFold(filepath.Clean(absCandidate), filepath.Clean(absTop)) {
					res.Warnings = append(res.Warnings, fmt.Sprintf("using explicit --path %s instead of Git repository root %s", absCandidate, absTop))
				}
			}
		}
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return res, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return res, apperr.WithPath(apperr.Usage("invalid_project_path", "project path does not exist"), abs)
		}
		return res, err
	}
	if IsReparseOrSymlink(info) {
		return res, apperr.WithPath(apperr.Usage("symlink_root", "project root cannot be a symbolic link or reparse point"), abs)
	}
	if !info.IsDir() {
		return res, apperr.WithPath(apperr.Usage("invalid_project_path", "project path is not a directory"), abs)
	}
	if filepath.Dir(abs) == abs {
		return res, apperr.WithPath(apperr.Usage("filesystem_root", "filesystem root cannot be used as a project"), abs)
	}
	if home, err := env.UserHomeDir(); err == nil && home != "" {
		if absHome, herr := filepath.Abs(home); herr == nil && strings.EqualFold(filepath.Clean(abs), filepath.Clean(absHome)) {
			return res, apperr.WithPath(apperr.Usage("home_as_project", "home directory cannot be used as a project; pass --global instead"), abs)
		}
	}
	res.Root = abs
	return res, nil
}

// ResolveUserHome resolves only user-home for global runtimes that do not use a runtime-specific home override.
func ResolveUserHome(env Environment) (GlobalResolution, error) {
	env = env.withDefaults()
	var res GlobalResolution
	absHome, err := resolveUserHome(env)
	if err != nil {
		return res, err
	}
	res.UserHome = absHome
	return res, nil
}

// ResolveGlobal resolves user-home and codex-home for global Codex operations.
func ResolveGlobal(env Environment) (GlobalResolution, error) {
	env = env.withDefaults()
	var res GlobalResolution
	absHome, err := resolveUserHome(env)
	if err != nil {
		return res, err
	}
	codexHome, warnings, err := resolveEnvDirectory(env, "CODEX_HOME", filepath.Join(absHome, ".codex"), "invalid_codex_home")
	if err != nil {
		return res, err
	}
	res.UserHome = absHome
	res.CodexHome = codexHome
	res.Warnings = append(res.Warnings, warnings...)
	return res, nil
}

// ResolveGeminiGlobal resolves user-home and gemini-home ($GEMINI_CLI_HOME/.gemini
// or ~/.gemini) for global Gemini CLI operations.
func ResolveGeminiGlobal(env Environment) (GlobalResolution, error) {
	env = env.withDefaults()
	var res GlobalResolution
	absHome, err := resolveUserHome(env)
	if err != nil {
		return res, err
	}
	cliBase, warnings, err := resolveEnvDirectory(env, "GEMINI_CLI_HOME", absHome, "invalid_gemini_home")
	if err != nil {
		return res, err
	}
	geminiHome := filepath.Clean(filepath.Join(cliBase, ".gemini"))
	if err := checkOptionalDirectory(geminiHome, "invalid_gemini_home", "GEMINI_CLI_HOME/.gemini"); err != nil {
		return res, err
	}
	res.UserHome = absHome
	res.GeminiHome = geminiHome
	res.Warnings = append(res.Warnings, warnings...)
	return res, nil
}

// ResolveOpenCodeGlobal resolves user-home and opencode-home
// ($OPENCODE_CONFIG_DIR, $XDG_CONFIG_HOME/opencode, or ~/.config/opencode).
func ResolveOpenCodeGlobal(env Environment) (GlobalResolution, error) {
	env = env.withDefaults()
	var res GlobalResolution
	absHome, err := resolveUserHome(env)
	if err != nil {
		return res, err
	}
	var ocHome string
	if rawDir := env.Getenv("OPENCODE_CONFIG_DIR"); rawDir != "" {
		dir, warnings, err := resolveEnvDirectory(env, "OPENCODE_CONFIG_DIR", "", "invalid_opencode_home")
		if err != nil {
			return res, err
		}
		ocHome = dir
		res.Warnings = append(res.Warnings, warnings...)
	} else if xdg := env.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		base, warnings, err := resolveEnvDirectory(env, "XDG_CONFIG_HOME", "", "invalid_opencode_home")
		if err != nil {
			return res, err
		}
		ocHome = filepath.Clean(filepath.Join(base, "opencode"))
		if err := checkOptionalDirectory(ocHome, "invalid_opencode_home", "XDG_CONFIG_HOME/opencode"); err != nil {
			return res, err
		}
		res.Warnings = append(res.Warnings, warnings...)
	} else {
		ocHome = filepath.Clean(filepath.Join(absHome, ".config", "opencode"))
		if err := checkOptionalDirectory(ocHome, "invalid_opencode_home", "~/.config/opencode"); err != nil {
			return res, err
		}
	}
	res.UserHome = absHome
	res.OpenCodeHome = ocHome
	return res, nil
}

// ResolveClaudeGlobal resolves user-home and claude-home ($CLAUDE_CONFIG_DIR or ~/.claude)
// for global Claude Code operations.
func ResolveClaudeGlobal(env Environment) (GlobalResolution, error) {
	env = env.withDefaults()
	var res GlobalResolution
	absHome, err := resolveUserHome(env)
	if err != nil {
		return res, err
	}
	claudeHome, warnings, err := resolveEnvDirectory(env, "CLAUDE_CONFIG_DIR", filepath.Join(absHome, ".claude"), "invalid_claude_home")
	if err != nil {
		return res, err
	}
	res.UserHome = absHome
	res.ClaudeHome = claudeHome
	res.Warnings = append(res.Warnings, warnings...)
	return res, nil
}

func resolveUserHome(env Environment) (string, error) {
	home, err := env.UserHomeDir()
	if err != nil {
		return "", err
	}
	absHome, err := filepath.Abs(home)
	if err != nil {
		return "", err
	}
	homeInfo, err := os.Lstat(absHome)
	if err != nil {
		return "", err
	}
	if IsReparseOrSymlink(homeInfo) || !homeInfo.IsDir() {
		return "", apperr.WithPath(apperr.Usage("invalid_user_home", "user home directory cannot be a symbolic link or non-directory"), absHome)
	}
	return absHome, nil
}

func resolveEnvDirectory(env Environment, envVar, defaultPath, errCode string) (string, []string, error) {
	var warnings []string
	raw := env.Getenv(envVar)
	target := raw
	if target == "" {
		target = defaultPath
	} else if !filepath.IsAbs(target) {
		absTarget, err := filepath.Abs(target)
		if err != nil {
			return "", nil, err
		}
		warnings = append(warnings, fmt.Sprintf("%s is relative (%q); resolved to %s (depends on working directory)", envVar, raw, absTarget))
		target = absTarget
	}
	target = filepath.Clean(target)
	if err := checkOptionalDirectory(target, errCode, envVar); err != nil {
		return "", nil, err
	}
	return target, warnings, nil
}

func checkOptionalDirectory(dir, errCode, label string) error {
	info, err := os.Lstat(dir)
	if err == nil {
		if IsReparseOrSymlink(info) || !info.IsDir() {
			return apperr.WithPath(apperr.Usage(errCode, fmt.Sprintf("%s cannot be a symbolic link or non-directory", label)), dir)
		}
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ValidateRelativePath checks that a slash-separated relative path is clean,
// has depth < 12, and contains no Windows reserved device names or special characters.
func ValidateRelativePath(p string) bool {
	if !fs.ValidPath(p) || p == "." || strings.ContainsAny(p, `\:`) || strings.Count(p, "/") >= 12 {
		return false
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == "" || strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
			return false
		}
		stem := strings.ToUpper(strings.SplitN(segment, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || reservedDevicePattern.MatchString(stem) {
			return false
		}
	}
	return true
}

// IsReparseOrSymlink detects symbolic links and non-regular reparse points (such as Windows junctions).
func IsReparseOrSymlink(info os.FileInfo) bool {
	return info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0
}

// ResolveTarget resolves relative inside root while rejecting path escapes,
// unsafe Windows segments, and symlinks/reparse points at root or beneath root.
func ResolveTarget(root, relative string) (string, error) {
	if root == "" || !ValidateRelativePath(relative) {
		return "", apperr.WithPath(apperr.Usage("invalid_target_path", "invalid root or relative path"), relative)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absRoot = filepath.Clean(absRoot)
	target := filepath.Join(absRoot, filepath.FromSlash(relative))
	inside, err := filepath.Rel(absRoot, target)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
		return "", apperr.WithPath(apperr.Usage("path_escape", "target path escapes root"), relative)
	}
	for current := target; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil && IsReparseOrSymlink(info) {
			return "", apperr.WithPath(apperr.Conflict("symlink_in_path", "symbolic link or reparse point in target path"), current)
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if current == absRoot {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
	}
	return target, nil
}
