//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package term

import "io"

func Size(io.Writer) (int, bool) { return 0, false }

func Dimensions(io.Writer) (int, int, bool) { return 0, 0, false }
