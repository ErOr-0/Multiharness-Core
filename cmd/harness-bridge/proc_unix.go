//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// startDetached starts the worker in its own session so it outlives the shell
// call that started it.
func startDetached(newCmd func() *exec.Cmd) (*exec.Cmd, error) {
	cmd := newCmd()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

// prepareHarness decides how the harness stops on cancel or timeout. A
// detached worker gives the harness its own process group and stops all of
// it. In the foreground the harness stays in the caller's group, so whatever
// stops the caller stops the harness too.
func prepareHarness(cmd *exec.Cmd, detached bool) {
	if !detached {
		cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
}
