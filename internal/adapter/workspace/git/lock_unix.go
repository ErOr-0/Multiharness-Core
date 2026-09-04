//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package git

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

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
