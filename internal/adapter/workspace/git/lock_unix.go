//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package git

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

func acquireLock(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("open repository lock: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("inspect repository lock: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("repository lock must be a regular file")
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrBusy
		}
		return nil, fmt.Errorf("lock repository: %w", err)
	}
	// Never unlink this file: replacing its inode would let another process
	// acquire a different lock while an existing run is still using this one.
	return file, nil
}
