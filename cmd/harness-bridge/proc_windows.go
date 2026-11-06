//go:build windows

package main

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// removeAfterExit deletes dir a moment after this process exits, from a
// detached cmd.exe, because Windows doesn't let a running program delete its
// own file. Only a folder named harness-bridge, with no characters cmd.exe
// treats specially, is accepted.
func removeAfterExit(dir string) error {
	if filepath.Base(dir) != "harness-bridge" || strings.ContainsAny(dir, "\"%&|<>^!") {
		return fmt.Errorf("can't safely remove %s automatically; delete it yourself", dir)
	}
	line := `cmd.exe /d /c ping -n 3 127.0.0.1 >nul & rmdir /s /q "` + dir + `"`
	start := func(flags uint32) error {
		cmd := exec.Command("cmd.exe")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags, CmdLine: line}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
	err := start(createNewProcessGroup | createNoWindow | createBreakawayFromJob)
	if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		err = start(createNewProcessGroup | createNoWindow)
	}
	return err
}

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
