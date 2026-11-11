//go:build windows

package agent

import (
	"os/exec"
	"strconv"
	"syscall"
)

// prepareHarness stops the harness together with its children: CLIs installed
// through npm run as cmd.exe, then node, then the agent binary.
func prepareHarness(cmd *exec.Cmd, _ bool) {
	cmd.Cancel = func() error {
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if kill.Run() != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
