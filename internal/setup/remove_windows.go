//go:build windows

package setup

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// removeAfterExit deletes dir a moment after this process exits, from a
// detached cmd.exe, because Windows doesn't let a running program delete its
// own file. Only a folder named harness-bridge, with no characters cmd.exe
// treats specially, is accepted.
func removeAfterExit(dir string) error {
	const (
		createNewProcessGroup  = 0x00000200
		createBreakawayFromJob = 0x01000000
		createNoWindow         = 0x08000000
	)
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
