//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package term

import (
	"io"

	"golang.org/x/sys/unix"
)

// Size reports the width of writer and whether it is a terminal.
func Size(writer io.Writer) (int, bool) {
	width, _, ok := Dimensions(writer)
	return width, ok
}

// Dimensions reports the columns and rows of writer and whether it is a terminal.
func Dimensions(writer io.Writer) (int, int, bool) {
	file, ok := writer.(interface{ Fd() uintptr })
	if !ok {
		return 0, 0, false
	}
	size, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, false
	}
	return int(size.Col), int(size.Row), true
}
