// Package hooks implements DTKit v0.2 opt-in Codex hook command formatting,
// structural JSON merging, and read-only session context handling.
package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
	"github.com/truong-newbie/Agent_Kit_Impl/internal/paths"
)

// IsValidExecutable reports whether p is a non-ephemeral, regular, non-symlink executable
// (and has at least one execute permission bit set on POSIX systems).
func IsValidExecutable(p string) bool {
	if p == "" || !filepath.IsAbs(p) {
		return false
	}
	clean := filepath.Clean(p)
	if isEphemeralExecutablePath(clean) {
		return false
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return false
	}
	return validateExecutableInfo(clean, info) == nil
}

func validateExecutableInfo(clean string, info os.FileInfo) error {
	if isEphemeralExecutablePath(clean) {
		return apperr.WithPath(apperr.Usage("ephemeral_executable", "hook executable cannot reside in a temporary build or test binary path"), clean)
	}
	if paths.IsReparseOrSymlink(info) || !info.Mode().IsRegular() {
		return apperr.WithPath(apperr.Usage("invalid_executable", "executable must be a regular file (not symlink or directory)"), clean)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return apperr.WithPath(apperr.Usage("executable_not_executable", "executable lacks POSIX execute permission"), clean)
	}
	return nil
}

// ResolveStableExecutable finds a non-ephemeral dtkit binary path for native hooks.
// Temporary go-run/go-build paths and test binaries are rejected so hooks do not break after cleanup.
func ResolveStableExecutable(explicit string, executableFn func() (string, error), lookPathFn func(string) (string, error)) (string, error) {
	if explicit != "" {
		if !filepath.IsAbs(explicit) {
			return "", apperr.WithPath(apperr.Usage("invalid_executable", "--executable must be an absolute path"), explicit)
		}
		clean := filepath.Clean(explicit)
		info, err := os.Lstat(clean)
		if err != nil {
			return "", apperr.WithPath(apperr.Usage("invalid_executable", fmt.Sprintf("executable not accessible: %v", err)), clean)
		}
		if vErr := validateExecutableInfo(clean, info); vErr != nil {
			return "", vErr
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
		clean := filepath.Clean(abs)
		info, err := os.Lstat(clean)
		if err == nil && validateExecutableInfo(clean, info) == nil {
			return clean, nil
		}
	}
	return "", apperr.WithHint(
		apperr.Usage("unstable_executable", "cannot locate a stable executable dtkit binary"),
		"pass --executable <absolute-path> pointing to an installed executable dtkit binary",
	)
}

func isEphemeralExecutablePath(p string) bool {
	lower := strings.ToLower(filepath.ToSlash(p))
	return strings.Contains(lower, "/go-build") || strings.Contains(lower, "\\go-build") || strings.HasSuffix(lower, ".test") || strings.HasSuffix(lower, ".test.exe")
}

// QuotePOSIX returns a single-quoted POSIX shell token safe for arbitrary paths.
func QuotePOSIX(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// QuotePowerShell returns a single-quoted PowerShell literal safe for paths with spaces or special chars.
func QuotePowerShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// HookSpec describes a DTKit-managed hook handler for a specific runtime.
type HookSpec struct {
	ID            string
	Event         string
	Matcher       string
	TimeoutSec    int
	StatusMessage string
}

// ResolveHookSpec validates and returns metadata for (runtimeID, hookID).
func ResolveHookSpec(runtimeID, hookID string) (HookSpec, error) {
	switch runtimeID {
	case "", "codex":
		if hookID == "session-context" {
			return HookSpec{
				ID:            "session-context",
				Event:         EventSessionStart,
				Matcher:       "startup|resume|clear|compact",
				TimeoutSec:    DefaultTimeoutSec,
				StatusMessage: "DTKit session context",
			}, nil
		}
		return HookSpec{}, apperr.Usage("unsupported_hook", fmt.Sprintf("unsupported Codex hook %q (supported: session-context)", hookID))
	case "claude":
		switch hookID {
		case "check-changed":
			return HookSpec{
				ID:            "check-changed",
				Event:         EventPostToolUse,
				Matcher:       "Edit|Write|MultiEdit",
				TimeoutSec:    15,
				StatusMessage: "DTKit changed-files quality check",
			}, nil
		case "guard-edit":
			return HookSpec{
				ID:            "guard-edit",
				Event:         EventPreToolUse,
				Matcher:       "Edit|Write|MultiEdit",
				TimeoutSec:    5,
				StatusMessage: "DTKit path/secret edit guard",
			}, nil
		default:
			return HookSpec{}, apperr.Usage("unsupported_hook", fmt.Sprintf("unsupported Claude hook %q (supported: check-changed, guard-edit)", hookID))
		}
	default:
		return HookSpec{}, apperr.Usage("hooks_not_supported", fmt.Sprintf("hooks are not supported for runtime %q", runtimeID))
	}
}

// FormatCommands renders OS-specific native hook commands for Codex hooks.json.
func FormatCommands(executable, hookID, scope, stateRoot string) (posixCmd, winCmd string, err error) {
	return FormatCommandsForRuntime("codex", executable, hookID, scope, stateRoot)
}

// FormatCommandsForRuntime renders shell-quoted native commands for the named runtime hook handler.
func FormatCommandsForRuntime(runtimeID, executable, hookID, scope, stateRoot string) (posixCmd, winCmd string, err error) {
	if _, err := ResolveHookSpec(runtimeID, hookID); err != nil {
		return "", "", err
	}
	if scope != "project" && scope != "global" {
		return "", "", apperr.Usage("invalid_scope", fmt.Sprintf("invalid scope %q", scope))
	}
	if !filepath.IsAbs(executable) || !filepath.IsAbs(stateRoot) {
		return "", "", apperr.Usage("invalid_hook_paths", "hook executable and root must be absolute paths")
	}
	rtFlag := ""
	if runtimeID != "" && runtimeID != "codex" {
		rtFlag = " --runtime " + runtimeID
	}
	posixCmd = fmt.Sprintf("%s hook run %s --scope %s --root %s%s",
		QuotePOSIX(executable), hookID, scope, QuotePOSIX(stateRoot), rtFlag)
	winCmd = fmt.Sprintf("& %s hook run %s --scope %s --root %s%s",
		QuotePowerShell(executable), hookID, scope, QuotePowerShell(stateRoot), rtFlag)
	return posixCmd, winCmd, nil
}
