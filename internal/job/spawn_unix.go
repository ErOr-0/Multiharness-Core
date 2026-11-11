//go:build !windows

package job

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
