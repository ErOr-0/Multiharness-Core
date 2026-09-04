//go:build windows

package git

import (
	"fmt"
	"os"
	"path/filepath"
)

const snapshotReadFlags = 0

func checkWorkspaceAccess(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", path)
	}
	probe := filepath.Join(path, fmt.Sprintf(".multiharness_access_probe_%d", os.Getpid()))
	if err := os.WriteFile(probe, []byte{}, 0600); err != nil {
		return fmt.Errorf("directory write permission denied: %w", err)
	}
	_ = os.Remove(probe)
	return nil
}
