//go:build windows

package paths

import (
	"errors"
	"syscall"
)

const (
	processQueryLimitedInfo = 0x1000
	stillActive             = 259
	errInvalidParameter     = syscall.Errno(87)
	errAccessDenied         = syscall.Errno(5)
)

// ProcessLiveness checks whether pid is alive on Windows ("alive", "dead", or "unknown").
func ProcessLiveness(pid int) string {
	if pid <= 0 {
		return "unknown"
	}
	handle, err := syscall.OpenProcess(processQueryLimitedInfo, false, uint32(pid))
	if err != nil {
		if errors.Is(err, errInvalidParameter) {
			return "dead"
		}
		if errors.Is(err, errAccessDenied) {
			return "alive"
		}
		return "unknown"
	}
	defer syscall.CloseHandle(handle)
	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return "unknown"
	}
	if exitCode == stillActive {
		return "alive"
	}
	return "dead"
}
