package process

import (
	"fmt"
	"os"
)

func validateWorkingDirectory(path string) error {
	if path == "" {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("access working directory %q: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory %q is not a directory", path)
	}
	return nil
}
