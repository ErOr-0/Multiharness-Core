//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import "golang.org/x/sys/unix"

const secretGetTermios = unix.TIOCGETA
const secretSetTermios = unix.TIOCSETA
