//go:build !windows

package setup

// removeAfterExit deletes dir now: unlike Windows, a running program's file
// can be deleted.
func removeAfterExit(dir string) error { return removeOurs(dir) }
