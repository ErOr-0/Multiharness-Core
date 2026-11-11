//go:build !windows

package app

import (
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/testkit"
)

// A timeout or cancel must stop the whole harness process tree, not just the
// direct child, or the agent's own children keep running.
func TestTimeoutStopsTheProcessTree(t *testing.T) {
	t.Setenv(testkit.FakeEnv, "codex-tree")
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("HARNESS_BRIDGE_TREE_PID", pidPath)
	r, code := testkit.Bridge(t, Run, "run", "--to", "codex", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "x", "--timeout", "1s")
	if code != cli.ExitFailed || r.Status != agent.StatusTimedOut {
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
