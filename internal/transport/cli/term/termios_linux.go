//go:build linux

package term

import "golang.org/x/sys/unix"

// Request codes for reading and replacing terminal attributes.
const GetTermios = unix.TCGETS
const SetTermios = unix.TCSETS
