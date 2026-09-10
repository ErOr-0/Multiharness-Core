//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package folder

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFolderLockHelper(t *testing.T) {
	dir := os.Getenv("MULTIHARNESS_FOLDER_LOCK_HELPER")
	if dir == "" {
		return
	}
	locks, err := acquireFolderLocks(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closeLocks(locks)
	os.Stdout.WriteString("ready\n")
	io.Copy(io.Discard, os.Stdin)
}
func TestFolderLockCrossesProcessesAndReleasesAfterCrash(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFolderLockHelper$")
	cmd.Env = append(os.Environ(), "MULTIHARNESS_FOLDER_LOCK_HELPER="+dir)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	ready := make([]byte, 6)
	if _, err := io.ReadFull(output, ready); err != nil {
		t.Fatal(err)
	}
	if locks, err := acquireFolderLocks(dir); !errors.Is(err, ErrBusy) {
		closeLocks(locks)
		t.Fatalf("lock not held: %v", err)
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	locks, err := acquireFolderLocks(dir)
	if err != nil {
		t.Fatal(err)
	}
	closeLocks(locks)
}
