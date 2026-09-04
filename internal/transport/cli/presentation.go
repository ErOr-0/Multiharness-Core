package cli

import (
	"crypto/rand"
	"encoding/json"
	"io"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/store"
)

// Result is the versioned CLI envelope. Correlation belongs to delivery, not to
// agent-facing domain contracts. Each invocation is a new task/run; repair
// attempts share these IDs. Neither ID is an agent session credential.
type Result struct {
	SchemaVersion string `json:"schema_version"`
	TaskID        string `json:"task_id"`
	RunID         string `json:"run_id"`
	store.TaskOutput
}

type presentation struct {
	stdout   io.Writer
	progress *progressSink
}

func newPresentation(stdout, stderr io.Writer) *presentation {
	return &presentation{stdout: stdout, progress: &progressSink{
		writer: stderr, format: "text", taskID: "task_" + rand.Text(), runID: "run_" + rand.Text(),
		pending: make(chan activity.Event, 1),
	}}
}

func (p *presentation) fail(message string, code int) int {
	failureCode := store.FailureCodeInvalidInput
	if code == ExitFailed {
		failureCode = store.FailureCodeInternal
	}
	return p.finish(store.TaskOutput{Status: store.TaskStatusFailed, Summary: "workflow could not start", Failure: &store.TaskFailure{Stage: store.WorkflowStageIntake, Code: failureCode, Message: message}}, code)
}

func (p *presentation) finish(output store.TaskOutput, code int) int {
	p.progress.result(output, code)
	if err, stage := p.progress.failure(); err != nil {
		output.Status, output.Summary, code = store.TaskStatusFailed, "workflow progress could not be written", ExitFailed
		output.Failure = &store.TaskFailure{Stage: stage, Code: store.FailureCodeInternal, Message: "progress writer failed"}
	}
	result := Result{SchemaVersion: "1", TaskID: p.progress.taskID, RunID: p.progress.runID, TaskOutput: output}
	data, err := json.Marshal(result)
	if err != nil {
		return ExitFailed
	}
	data = append(data, '\n')
	if n, err := p.stdout.Write(data); err != nil || n != len(data) {
		// Do not echo the writer's error: it may contain arbitrary sensitive text.
		p.progress.resultDeliveryFailed()
		return ExitFailed
	}
	return code
}
