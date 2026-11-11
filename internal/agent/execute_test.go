package agent_test

import (
	"testing"
	"time"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/app"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/testkit"
)

// A turn whose final success event arrived must be reported at once, even
// when the harness's process (an MCP server it started) stays alive.
func TestFinishedTurnOutlivesItsProcess(t *testing.T) {
	old := *agent.ExitGrace
	*agent.ExitGrace = time.Second
	t.Cleanup(func() { *agent.ExitGrace = old })
	t.Setenv(testkit.FakeEnv, "claude-hangs")
	started := time.Now()
	r, code := testkit.Bridge(t, app.Run, "run", "--to", "claude", "--bin", testkit.TestExe(t), "--dir", t.TempDir(), "--prompt", "x", "--timeout", "30s")
	if code != cli.ExitOK || r.Status != agent.StatusCompleted || r.Message != "Done" {
		t.Fatalf("exit %d, report %+v", code, r)
	}
	if elapsed := time.Since(started); elapsed > 15*time.Second {
		t.Fatalf("a finished turn took %s to report", elapsed)
	}
}
