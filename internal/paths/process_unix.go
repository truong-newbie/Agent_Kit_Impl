//go:build !windows

package paths

import (
	"errors"
	"syscall"
)

// ProcessLiveness checks whether pid is alive on POSIX systems ("alive", "dead", or "unknown").
func ProcessLiveness(pid int) string {
	if pid <= 0 {
		return "unknown"
	}
	err := syscall.Kill(pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return "alive"
	}
	if errors.Is(err, syscall.ESRCH) {
		return "dead"
	}
	return "unknown"
}
