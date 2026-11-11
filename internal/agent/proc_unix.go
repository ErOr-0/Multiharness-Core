//go:build !windows

package agent

import (
	"os/exec"
	"syscall"
	"time"
)

// prepareHarness decides how the harness stops on cancel or timeout. It runs
// in its own process group and Cancel signals the whole group, because a
// detached worker must not take the worker's group down with it, and in the
// foreground the CLI is often a wrapper (npm, a shell) whose children — the
// real binary, its test servers — would otherwise survive. The group also
// means the terminal's Ctrl-C no longer reaches the harness, so the run
// command cancels its own context on SIGINT/SIGTERM and stops the tree
// itself.
func prepareHarness(cmd *exec.Cmd, _ bool) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		// WaitDelay's final kill only reaches the direct child, so give the
		// group a moment to exit on its own and then kill it for real.
		pid := cmd.Process.Pid
		go func() {
			time.Sleep(5 * time.Second)
			_ = syscall.Kill(-pid, syscall.SIGKILL) // nothing left when it already exited
		}()
		return err
	}
}
