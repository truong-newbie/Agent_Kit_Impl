// Package hooks implements DTKit v0.2 opt-in Codex hook command formatting,
// structural JSON merging, and read-only session context handling.
package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// ResolveStableExecutable finds a non-ephemeral dtkit binary path for native hooks.
// Temporary go-run/go-build paths are rejected so hooks do not break after cleanup.
func ResolveStableExecutable(explicit string, executableFn func() (string, error), lookPathFn func(string) (string, error)) (string, error) {
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			return "", apperr.WithPath(apperr.Usage("invalid_executable", "--executable must be an absolute path"), explicit)
		}
		clean := filepath.Clean(explicit)
		if strings.Contains(strings.ToLower(filepath.ToSlash(clean)), "/go-build") {
			return "", apperr.WithPath(apperr.Usage("ephemeral_executable", "hook executable cannot reside in a temporary go build directory"), clean)
		}
		info, err := os.Lstat(clean)
		if err != nil {
			return "", apperr.WithPath(apperr.Usage("invalid_executable", fmt.Sprintf("executable not accessible: %v", err)), clean)
		}
		if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
			return "", apperr.WithPath(apperr.Usage("invalid_executable", "executable must be a regular file (not symlink or directory)"), clean)
		}
		return clean, nil
	}

	var candidates []string
	if executableFn != nil {
		if exe, err := executableFn(); err == nil && exe != "" {
			candidates = append(candidates, exe)
		}
	}
	if lookPathFn != nil {
		if p, err := lookPathFn("dtkit"); err == nil && p != "" {
			candidates = append(candidates, p)
		}
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		if isEphemeralBuildPath(abs) || strings.HasSuffix(strings.ToLower(abs), ".test") || strings.HasSuffix(strings.ToLower(abs), ".test.exe") {
			continue
		}
		info, err := os.Lstat(abs)
		if err == nil && !paths.IsReparseOrSymlink(info) && info.Mode().IsRegular() {
			return abs, nil
		}
	}
	return "", apperr.WithHint(
		apperr.Usage("unstable_executable", "cannot locate a stable installed dtkit executable"),
		"pass --executable <absolute-path> pointing to an installed dtkit binary",
	)
}

func isEphemeralBuildPath(p string) bool {
	lower := strings.ToLower(filepath.ToSlash(p))
	return strings.Contains(lower, "/go-build") || strings.Contains(lower, "\\go-build")
}

// QuotePOSIX returns a single-quoted POSIX shell token safe for arbitrary paths.
func QuotePOSIX(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// QuotePowerShell returns a single-quoted PowerShell literal safe for paths with spaces or special chars.
func QuotePowerShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// FormatCommands renders OS-specific native hook commands for Codex hooks.json.
func FormatCommands(executable, hookID, scope, stateRoot string) (posixCmd, winCmd string, err error) {
	if hookID != "session-context" {
		return "", "", apperr.Usage("unsupported_hook", fmt.Sprintf("unsupported hook %q", hookID))
	}
	if scope != "project" && scope != "global" {
		return "", "", apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	if !filepath.IsAbs(executable) || !filepath.IsAbs(stateRoot) {
		return "", "", apperr.Usage("invalid_hook_paths", "hook executable and root must be absolute paths")
	}
	posixCmd = fmt.Sprintf("%s hook run %s --scope %s --root %s",
		QuotePOSIX(executable), hookID, scope, QuotePOSIX(stateRoot))
	winCmd = fmt.Sprintf("& %s hook run %s --scope %s --root %s",
		QuotePowerShell(executable), hookID, scope, QuotePowerShell(stateRoot))
	return posixCmd, winCmd, nil
}
