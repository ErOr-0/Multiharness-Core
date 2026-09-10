//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package folder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Shared ancestor locks and an exclusive target lock prevent parent/child runs
// from overlapping, while independent sibling folders can run concurrently.
// Directory inodes are shared through bind mounts; no lock file or Git repository
// is created in a plain workspace. The OS releases locks after a process crash.
func acquireFolderLocks(root string) ([]*os.File, error) {
	var locks []*os.File
	for dir := root; ; dir = filepath.Dir(dir) {
		fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("open workspace lock: %w", err), closeLocks(locks))
		}
		file := os.NewFile(uintptr(fd), dir)
		locks = append(locks, file)
		mode := unix.LOCK_SH
		if dir == root {
			mode = unix.LOCK_EX
		}
		if err := unix.Flock(fd, mode|unix.LOCK_NB); err != nil {
			if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
				err = ErrBusy
			}
			return nil, errors.Join(fmt.Errorf("lock workspace: %w", err), closeLocks(locks))
		}
		if filepath.Dir(dir) == dir {
			return locks, nil
		}
	}
}
