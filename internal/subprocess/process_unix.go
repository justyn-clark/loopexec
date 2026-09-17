//go:build darwin || linux

package subprocess

import (
	"os/exec"
	"syscall"
)

const ProcessGroups = true

func configure(c *exec.Cmd, join bool) {
	if !join {
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
}
func terminate(c *exec.Cmd) {
	if c.SysProcAttr != nil && c.SysProcAttr.Setpgid {
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGTERM)
	} else {
		_ = c.Process.Signal(syscall.SIGTERM)
	}
}
func kill(c *exec.Cmd) {
	if c.SysProcAttr != nil && c.SysProcAttr.Setpgid {
		_ = syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
	} else {
		_ = c.Process.Kill()
	}
}
