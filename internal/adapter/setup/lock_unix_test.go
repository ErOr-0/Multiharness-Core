//go:build linux || darwin

package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupLockExcludesConcurrentInstalls(t *testing.T) {
	dir := t.TempDir()
	// t.TempDir may inherit a permissive umask; our lock directory is private.
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := lockDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := lockDirectory(dir)
	if err == nil {
		second.Close()
		t.Fatal("concurrent installation permitted")
	}
	first.Close()
	second, err = lockDirectory(dir)
	if err != nil {
		t.Fatal("lock not released", err)
	}
	second.Close()
}

func TestSetupLockRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "user-data")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "install.lock")); err != nil {
		t.Fatal(err)
	}
	if lease, err := lockDirectory(dir); err == nil {
		lease.Close()
		t.Fatal("accepted symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep" {
		t.Fatal("changed user file")
	}
}
