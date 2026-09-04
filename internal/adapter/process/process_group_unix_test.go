//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package process

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCancellationTerminatesProcessTree(t *testing.T) {
	pidFile := t.TempDir() + "/child.pid"
	ctx, cancel := context.WithCancel(context.Background())
	command := helperCommand(t, "tree-parent")
	command.Timeout = 0
	command.EnvOverrides[helperPIDFile] = pidFile

	done := make(chan error, 1)
	go func() {
		_, err := NewOSRunner().Run(ctx, command)
		done <- err
	}()

	childPID := waitForChildPID(t, pidFile)
	t.Cleanup(func() {
		_ = syscall.Kill(childPID, syscall.SIGKILL)
	})
	cancel()

	select {
	case err := <-done:
		assertRunErrorKind(t, err, ErrorKindCancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v; expected context cancellation", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for process-group cancellation")
	}

	deadline := time.Now().Add(3 * time.Second)
	for processExists(childPID) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if processExists(childPID) {
		t.Fatalf("child process %d is still running after cancellation", childPID)
	}
}

func waitForChildPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, conversionErr := strconv.Atoi(strings.TrimSpace(string(data)))
			if conversionErr != nil {
				t.Fatalf("invalid child PID %q: %v", data, conversionErr)
			}
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for child PID")
	return 0
}

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
