//go:build windows

package check

import (
	"os/exec"
	"strconv"
)

func configureCommandProcess(cmd *exec.Cmd) {}

func killProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	_ = cmd.Process.Kill()
}
