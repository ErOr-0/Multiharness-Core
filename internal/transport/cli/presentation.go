package cli

import (
	"encoding/json"
	"io"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/progress"
	"multiharness-core/internal/transport/cli/screen"
)

// Result is the versioned CLI envelope. Correlation belongs to delivery, not to
// agent-facing domain contracts. Each invocation is a new task/run; repair
// attempts share these IDs. Neither ID is an agent session credential.
type Result struct {
	SchemaVersion string `json:"schema_version"`
	TaskID        string `json:"task_id"`
	RunID         string `json:"run_id"`
	contract.TaskOutput
}

type presentation struct {
	output        contract.TaskOutput
	stdout        io.Writer
	progress      *progress.Sink
	human         *screen.View
	outputErr     error
	diagnosticDir string
}

func newPresentation(stdout, stderr io.Writer) *presentation {
	return &presentation{stdout: stdout, progress: progress.New(stderr)}
}

func (p *presentation) fail(message string, code int) int {
	failureCode := contract.FailureCodeInvalidInput
	if code == ExitFailed {
		failureCode = contract.FailureCodeInternal
	}
	return p.finish(
		contract.TaskOutput{
			Status:  contract.TaskStatusFailed,
			Summary: "workflow could not start",
			Failure: &contract.TaskFailure{Stage: contract.WorkflowStageIntake, Code: failureCode, Message: message},
		},
		code,
	)
}

func (p *presentation) finish(output contract.TaskOutput, code int) int {
	p.output = output
	p.progress.Result(output, code)
	if stage, err := p.progress.Failure(); err != nil {
		output.Status, output.Summary, code = contract.TaskStatusFailed, "workflow progress could not be written", ExitFailed
		output.Failure = &contract.TaskFailure{Stage: stage, Code: contract.FailureCodeInternal, Message: "progress writer failed"}
	}
	taskID, runID := p.progress.Correlation()
	result := Result{SchemaVersion: "1", TaskID: taskID, RunID: runID, TaskOutput: output}
	if p.human != nil {
		if p.diagnosticDir != "" && output.Failure != nil && output.Failure.Provider != nil {
			notice := "Provider diagnostics saved. Use /diagnostics to view the last failure."
			saveErr := saveProviderDiagnostic(p.diagnosticDir, result.RunID, output.Failure)
			if saveErr != nil {
				notice = "Could not save provider diagnostics; the provider details remain in this result."
			}
			if err := p.human.Notice(notice, saveErr != nil); err != nil {
				p.outputErr = err
				return ExitFailed
			}
		}
		if err := p.human.Result(output); err != nil {
			p.outputErr = err
			p.progress.ResultDeliveryFailed(ExitFailed)
			return ExitFailed
		}
		return code
	}
	data, err := json.Marshal(result)
	if err != nil {
		return ExitFailed
	}
	data = append(data, '\n')
	if n, err := p.stdout.Write(data); err != nil || n != len(data) {
		// Do not echo the writer's error: it may contain arbitrary sensitive text.
		p.progress.ResultDeliveryFailed(ExitFailed)
		return ExitFailed
	}
	return code
}
