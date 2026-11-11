package job

import (
	"context"

	"multiharness-core/internal/agent"
)

// Names the package's own external tests need.

const (
	LostConfirm   = lostConfirm
	WorkerLogFile = workerLogFile
)

var CreateJob = createJob

func (j *Job) Final(req agent.Request, r agent.Report) agent.Report { return j.final(req, r) }

func (j *Job) Watch(ctx context.Context, cancel context.CancelFunc, req agent.Request, t *agent.Tracker, stop <-chan struct{}) {
	j.watch(ctx, cancel, req, t, stop)
}
