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
	UserHome  string
	CodexHome string
	Warnings  []string
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

// ResolveGlobal resolves user-home and codex-home for global operations.
func ResolveGlobal(env Environment) (GlobalResolution, error) {
	env = env.withDefaults()
	var res GlobalResolution
	home, err := env.UserHomeDir()
	if err != nil {
		return res, err
	}
	absHome, err := filepath.Abs(home)
	if err != nil {
		return res, err
	}
	homeInfo, err := os.Lstat(absHome)
	if err != nil {
		return res, err
	}
	if IsReparseOrSymlink(homeInfo) || !homeInfo.IsDir() {
		return res, apperr.WithPath(apperr.Usage("invalid_user_home", "user home directory cannot be a symbolic link or non-directory"), absHome)
	}
	codexEnv := env.Getenv("CODEX_HOME")
	codexHome := codexEnv
	if codexHome == "" {
		codexHome = filepath.Join(absHome, ".codex")
	} else if !filepath.IsAbs(codexHome) {
		absCodex, err := filepath.Abs(codexHome)
		if err != nil {
			return res, err
		}
		res.Warnings = append(res.Warnings, fmt.Sprintf("CODEX_HOME is relative (%q); resolved to %s (depends on working directory)", codexEnv, absCodex))
		codexHome = absCodex
	}
	codexHome = filepath.Clean(codexHome)
	if info, err := os.Lstat(codexHome); err == nil {
		if IsReparseOrSymlink(info) || !info.IsDir() {
			return res, apperr.WithPath(apperr.Usage("invalid_codex_home", "CODEX_HOME cannot be a symbolic link or non-directory"), codexHome)
		}
	} else if !os.IsNotExist(err) {
		return res, err
	}
	res.UserHome = absHome
	res.CodexHome = codexHome
	return res, nil
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
