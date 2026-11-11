//go:build windows

package job

import (
	"errors"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup  = 0x00000200
	createBreakawayFromJob = 0x01000000
	createNoWindow         = 0x08000000
)

// startDetached starts the worker with its own hidden console, outside the
// caller's job object when the job allows it, so it outlives the shell call
// that started it. Its children share the hidden console, so no window opens.
func startDetached(newCmd func() *exec.Cmd) (*exec.Cmd, error) {
	flags := uint32(createNewProcessGroup | createNoWindow)
	cmd := newCmd()
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags | createBreakawayFromJob}
	err := cmd.Start()
	if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		// The caller's job object forbids breakaway; start inside it instead.
		cmd = newCmd()
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
		err = cmd.Start()
	}
	if err != nil {
		return nil, err
	}
	return cmd, nil
}
