//go:build !windows

package launcher

import (
	"fmt"
	"os"
	"strings"
)

func installPath(dir string) error {
	for _, entry := range strings.Split(os.Getenv("PATH"), ":") {
		if entry == dir {
			return nil
		}
	}
	return fmt.Errorf("add %s to your shell PATH, or run the installed launcher by its full path", dir)
}
