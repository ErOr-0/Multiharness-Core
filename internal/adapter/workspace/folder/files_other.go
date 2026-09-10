//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package folder

// Acquire fails closed on these platforms before any snapshot is read.
const snapshotReadFlags = 0

func checkWorkspaceAccess(string) error { return ErrUnsupported }
