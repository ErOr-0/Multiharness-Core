//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package git

import "golang.org/x/sys/unix"

// Reject a leaf replaced with a symlink; do not block if it becomes a FIFO
// between Lstat and Open. The opened file is also checked before reading.
const snapshotReadFlags = unix.O_NONBLOCK | unix.O_NOFOLLOW

func checkWorkspaceAccess(path string) error { return unix.Access(path, unix.R_OK|unix.W_OK|unix.X_OK) }
