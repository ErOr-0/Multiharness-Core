//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly && !windows

package git

import "os"

func acquireLock(string) (*os.File, error) { return nil, ErrUnsupported }
