//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package git

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
)

func TestRepositoryLockHelper(t *testing.T) {
	if os.Getenv("MULTIHARNESS_GIT_LOCK_HELPER") != "1" {
		return
	}
	workspace, err := NewWorkspace(process.NewOSRunner(), Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = workspace.Acquire(context.Background(), os.Getenv("MULTIHARNESS_TEST_REPO"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(os.Stdout, "locked")
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0) // Deliberately no Close: the OS must release it after process exit.
}

func TestRepositoryLockCrossesProcessesAndSurvivesOwnerCrash(t *testing.T) {
	dir := repository(t)
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestRepositoryLockHelper$")
	child.Env = append(os.Environ(), "MULTIHARNESS_GIT_LOCK_HELPER=1", "MULTIHARNESS_TEST_REPO="+dir)
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill() })
	ready := make(chan bool, 1)
	go func() { scanner := bufio.NewScanner(stdout); ready <- scanner.Scan() && scanner.Text() == "locked" }()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("lock helper did not become ready")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("lock helper timed out")
	}
	workspace := newWorkspace(t, Config{})
	if _, err := workspace.Acquire(t.Context(), dir); !errors.Is(err, ErrBusy) {
		t.Fatalf("cross-process acquisition: %v", err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = child.Wait()
	_ = acquire(t, workspace, dir)
}

func TestLinkedWorktreesShareOneWorkflowLock(t *testing.T) {
	dir := repository(t)
	linked := filepath.Join(t.TempDir(), "worktree")
	runGit(t, dir, "worktree", "add", "--detach", linked, "HEAD")
	_ = acquire(t, newWorkspace(t, Config{}), dir)
	if _, err := newWorkspace(t, Config{}).Acquire(t.Context(), linked); !errors.Is(err, ErrBusy) {
		t.Fatalf("linked-worktree lock: %v", err)
	}
}

func TestLockCannotFollowSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "lock")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if file, err := acquireLock(link); err == nil {
		_ = file.Close()
		t.Fatal("symlink lock accepted")
	}
	content, _ := os.ReadFile(target)
	if string(content) != "preserve" {
		t.Fatal("symlink target modified")
	}
}

func TestWorkspaceRequiresWritePermission(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	dir := repository(t)
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	if lease, err := newWorkspace(t, Config{}).Acquire(t.Context(), dir); err == nil {
		_ = lease.Close()
		t.Fatal("read-only workspace accepted")
	}
}
