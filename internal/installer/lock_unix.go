//go:build !windows

package installer

import (
	"errors"
	"syscall"
)

func processLiveness(pid int) string {
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
