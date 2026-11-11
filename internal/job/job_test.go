package job_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/agent"
	"multiharness-core/internal/app"
	"multiharness-core/internal/cli"
	"multiharness-core/internal/job"
	"multiharness-core/internal/jsonfile"
	"multiharness-core/internal/testkit"
)

func TestWaitReportsLostJob(t *testing.T) {
	req := agent.Request{Harness: "codex", Dir: t.TempDir(), Prompt: "x", Sandbox: "workspace-write", Timeout: time.Minute}
	j, err := job.CreateJob(req)
	if err != nil {
		t.Fatal(err)
	}
	stale := j.Final(req, agent.Report{Status: agent.StatusRunning})
	stale.UpdatedAt = time.Now().Add(-time.Minute)
	if err := jsonfile.Write(j.Path(job.StatusFile), stale); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	// The stale status must be confirmed for lostConfirm before the job is
	// given up on, so --max has to outlast it.
	r, code := testkit.Bridge(t, app.Run, "wait", j.ID, "--max", "10s")
	if code != cli.ExitFailed || r.Status != agent.StatusLost || !strings.Contains(r.Error, job.WorkerLogFile) {
		t.Fatalf("exit %d, report %+v", code, r)
	}
	if elapsed := time.Since(started); elapsed > job.LostConfirm+3*time.Second {
		t.Fatalf("reporting the lost job took %s", elapsed)
	}
}

// A stale status seen right after the machine wakes (or any clock jump) must
// not end the wait while the worker is still heartbeating.
func TestWaitSurvivesAStaleStatus(t *testing.T) {
	req := agent.Request{Harness: "codex", Dir: t.TempDir(), Prompt: "x", Sandbox: "workspace-write", Timeout: time.Minute}
	j, err := job.CreateJob(req)
	if err != nil {
		t.Fatal(err)
	}
	stale := j.Final(req, agent.Report{Status: agent.StatusRunning})
	stale.UpdatedAt = time.Now().Add(-10 * time.Minute)
	if err := jsonfile.Write(j.Path(job.StatusFile), stale); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(time.Second)
		fresh := j.Final(req, agent.Report{Status: agent.StatusRunning})
		fresh.UpdatedAt = time.Now()
		_ = jsonfile.Write(j.Path(job.StatusFile), fresh)
		_ = jsonfile.Write(j.Path(job.ResultFile), j.Final(req, agent.Report{Status: agent.StatusCompleted, Message: "done"}))
	}()
	r, code := testkit.Bridge(t, app.Run, "wait", j.ID, "--max", "20s")
	if code != cli.ExitOK || r.Status != agent.StatusCompleted || r.Message != "done" {
		t.Fatalf("exit %d, report %+v", code, r)
	}
}

// While the child is stopped and the result is being collected, the status
// file must keep being refreshed.
func TestWatchKeepsBeatingAfterCancel(t *testing.T) {
	req := agent.Request{Harness: "codex", Dir: t.TempDir(), Prompt: "x", Sandbox: "workspace-write", Timeout: time.Minute}
	j, err := job.CreateJob(req)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		j.Watch(ctx, cancel, req, agent.NewTracker(req), stop)
	}()
	readStamp := func() time.Time {
		st, err := job.ReadReport(j.Path(job.StatusFile))
		if err != nil {
			t.Fatal(err)
		}
		return st.UpdatedAt
	}
	time.Sleep(200 * time.Millisecond)
	first := readStamp()
	time.Sleep(1500 * time.Millisecond)
	second := readStamp()
	close(stop)
	<-done
	if !second.After(first) {
		t.Fatalf("status stopped changing after the cancel: %v then %v", first, second)
	}
}
