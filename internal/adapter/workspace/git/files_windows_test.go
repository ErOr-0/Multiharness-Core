//go:build windows

package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeWindowsFailsWithoutTouchingUserFiles(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, fmt.Sprintf(".multiharness_access_probe_%d", os.Getpid()))
	if err := os.WriteFile(name, []byte("user data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := checkWorkspaceAccess(dir); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unsafe platform accepted", err)
	}
	data, err := os.ReadFile(name)
	if err != nil || string(data) != "user data" {
		t.Fatal("access probe changed user data")
	}
}
