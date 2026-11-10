//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// A timeout or cancel must stop the whole harness process tree, not just the
// direct child, or the agent's own children keep running.
func TestTimeoutStopsTheProcessTree(t *testing.T) {
	t.Setenv(fakeEnv, "codex-tree")
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("HARNESS_BRIDGE_TREE_PID", pidPath)
	r, code := bridge(t, "run", "--to", "codex", "--bin", testExe(t), "--dir", t.TempDir(), "--prompt", "x", "--timeout", "1s")
	if code != exitFailed || r.Status != statusTimedOut {
		t.Fatalf("exit %d, report %+v", code, r)
	}
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); err == syscall.ESRCH {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the harness's child process %d is still running", pid)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
