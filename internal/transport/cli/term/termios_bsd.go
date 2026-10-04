//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package term

import "golang.org/x/sys/unix"

// Request codes for reading and replacing terminal attributes.
const GetTermios = unix.TIOCGETA
const SetTermios = unix.TIOCSETA
