//go:build !linux && !darwin

package setup

import (
	"errors"
	"io"
)

func installationLock() (io.Closer, error) {
	return nil, errors.New("automatic installation is unsupported on this platform")
}
