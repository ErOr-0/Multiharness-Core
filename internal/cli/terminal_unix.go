//go:build !windows

package cli

import "os"

// IsTerminal reports whether the file is a terminal. A char-device check alone
// also accepts a redirect from /dev/null, which would run the wizard with
// every answer taking its default.
func IsTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	null, err := os.Stat(os.DevNull)
	if err != nil {
		return false
	}
	return !os.SameFile(info, null)
}
