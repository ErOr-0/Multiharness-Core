//go:build windows

package cli

import (
	"os"
	"syscall"
)

// IsTerminal reports whether the file is a console. A char-device check alone
// also accepts redirects from NUL, which would run the wizard with every
// answer taking its default.
func IsTerminal(f *os.File) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode) == nil
}
