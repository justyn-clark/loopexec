//go:build !darwin && !linux

package subprocess

import "os/exec"

// Other platforms bound direct-child lifetimes only. Strict unattended
// process-tree containment is supported only on Linux and macOS.
const ProcessGroups = false

func configure(c *exec.Cmd, join bool) {}
func terminate(c *exec.Cmd)            { _ = c.Process.Kill() }
func kill(c *exec.Cmd)                 { _ = c.Process.Kill() }
