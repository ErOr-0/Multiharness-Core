//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package folder

import "os"

func acquireLock(string) (*os.File, error) { return nil, ErrUnsupported }

func acquireFolderLocks(string) ([]*os.File, error) { return nil, ErrUnsupported }
