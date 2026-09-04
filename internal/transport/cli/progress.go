package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// Logs have no free-text message field. Redaction is an allowlist, not a guess
// at credential formats: no prompt, path, diff, environment, agent output,
// session ID, or error string crosses this boundary. Unknown string metadata
// is replaced even if a custom event publisher violates the contract.
type logRecord struct {
	Version        int            `json:"version"`
	Time           string         `json:"time"`
	TaskID         string         `json:"task_id"`
	RunID          string         `json:"run_id"`
	Level          string         `json:"level"`
	Code           string         `json:"code,omitempty"`
	ExitCode       *int           `json:"exit_code,omitempty"`
	RuntimeVersion string         `json:"runtime_version,omitempty"`
	Agent          activity.Agent `json:"agent,omitempty"`
	Activity       activity.Kind  `json:"activity,omitempty"`
	workflow.Event
}

type progressSink struct {
	mu                    sync.Mutex
	writer                io.Writer
	format, taskID, runID string
	quiet, noChecks       bool
	cancel                context.CancelFunc
	err                   error
	stage                 store.WorkflowStage
	view                  liveView
	pending               chan activity.Event
	stopOnce              sync.Once
	stopCh, done          chan struct{}
}

func (p *progressSink) Publish(event workflow.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	event = redactEvent(event)
	p.beforeEvent(event, time.Now())
	p.stage = event.Stage
	level := "info"
	if event.Type == workflow.EventTypeStageFailed {
		level = "error"
	}
	p.write(logRecord{Level: level, Event: event})
	if p.noChecks && event.Type == workflow.EventTypeStageStarted && event.Stage == store.WorkflowStageValidation {
		p.write(logRecord{Level: "warning", Code: "no_validation_checks", Event: workflow.Event{Stage: event.Stage, Sequence: event.Sequence}})
	}
}

func (p *progressSink) result(output store.TaskOutput, code int) {
	p.stop()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.view.summary = resultSummary(output)
	p.write(logRecord{Level: "info", Code: "result_ready", ExitCode: &code, Event: redactEvent(workflow.Event{Status: output.Status})})
}

func (p *progressSink) resultDeliveryFailed() {
	p.mu.Lock()
	defer p.mu.Unlock()
	code := ExitFailed
	p.write(logRecord{Level: "error", Code: "result_output_failed", ExitCode: &code})
}

// A runtime-selection notice has no workflow sequence number and cannot change
// the workflow's stage. Only numeric release metadata reaches the log.
func (p *progressSink) CodexRuntimeSelected(version string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(version) > 32 || len(strings.Split(version, ".")) != 3 || strings.Trim(version, "0123456789.") != "" {
		version = "[redacted]"
	}
	p.write(logRecord{Level: "info", Code: "codex_runtime_selected", RuntimeVersion: version})
	return p.err
}

// write holds mu, keeping each JSONL record atomic for this run's publishers.
func (p *progressSink) write(record logRecord) {
	if p.quiet || p.err != nil {
		return
	}
	record.Version, record.Time = 1, time.Now().UTC().Format(time.RFC3339Nano)
	record.TaskID, record.RunID = p.taskID, p.runID
	var data []byte
	if p.format == "json" {
		data, p.err = json.Marshal(record)
		data = append(data, '\n')
	} else if p.view.friendly {
		p.writeHuman(record)
		return
	} else {
		var line strings.Builder
		fmt.Fprintf(&line, "task=%s run=%s ", p.taskID, p.runID)
		if record.Code == "" {
			fmt.Fprintf(&line, "[%d] %s %s", record.Sequence, record.Stage, record.Type)
		} else {
			fmt.Fprintf(&line, "%s=%s", record.Level, record.Code)
		}
		if record.RepairAttempt > 0 {
			fmt.Fprintf(&line, " attempt=%d", record.RepairAttempt)
		}
		if record.RetryAttempt > 0 {
			fmt.Fprintf(&line, " retry_attempt=%d provider_kind=%s agent_invocations=%d", record.RetryAttempt, record.ProviderKind, record.AgentInvocations)
			fmt.Fprintf(&line, " retry_delay_ms=%d", record.RetryDelayMillis)
		}
		if record.BlockingFindings > 0 {
			fmt.Fprintf(&line, " blocking_findings=%d", record.BlockingFindings)
		}
		if record.Status != "" {
			fmt.Fprintf(&line, " status=%s", record.Status)
		}
		if record.FailureCode != "" {
			fmt.Fprintf(&line, " failure_code=%s", record.FailureCode)
		}
		if record.Code == "no_validation_checks" {
			line.WriteString(" (no deterministic validation checks configured)")
		}
		if record.RuntimeVersion != "" {
			fmt.Fprintf(&line, " version=%s", record.RuntimeVersion)
		}
		if record.Activity != "" {
			fmt.Fprintf(&line, " agent=%s activity=%s", record.Agent, record.Activity)
		}
		if record.ExitCode != nil {
			fmt.Fprintf(&line, " exit_code=%d", *record.ExitCode)
		}
		line.WriteByte('\n')
		data = []byte(line.String())
	}
	p.writeBytes(data)
}

// All terminal writes, including animation and cleanup, share error handling.
func (p *progressSink) writeBytes(data []byte) {
	if p.quiet || p.err != nil {
		return
	}
	if p.err == nil {
		var n int
		n, p.err = p.writer.Write(data)
		if n != len(data) && p.err == nil {
			p.err = io.ErrShortWrite
		}
	}
	if p.err != nil && p.cancel != nil {
		p.cancel()
	}
}

func (p *progressSink) failure() (error, store.WorkflowStage) {
	p.mu.Lock()
	defer p.mu.Unlock()
	stage := p.stage
	if stage == "" || stage == "[redacted]" {
		stage = store.WorkflowStageIntake
	}
	return p.err, stage
}

func redactEvent(event workflow.Event) workflow.Event {
	if event.RetryDelayMillis < 0 || event.RetryDelayMillis > int64(24*time.Hour/time.Millisecond) {
		event.RetryDelayMillis = 0
	}
	switch event.Type {
	case "", workflow.EventTypeStageStarted, workflow.EventTypeStageProgress, workflow.EventTypeStageCompleted, workflow.EventTypeStageFailed, workflow.EventTypeWorkflowCompleted, workflow.EventTypeAgentRetryScheduled, workflow.EventTypeAgentSwitched:
	default:
		event.Type = "[redacted]"
	}
	switch event.Stage {
	case "", store.WorkflowStageIntake, store.WorkflowStagePlanning, store.WorkflowStageImplementation, store.WorkflowStageValidation, store.WorkflowStageReview, store.WorkflowStageRepair:
	default:
		event.Stage = "[redacted]"
	}
	switch event.Status {
	case "", store.TaskStatusAnswered, store.TaskStatusApproved, store.TaskStatusFailed, store.TaskStatusCancelled, store.TaskStatusRepairLimitReached:
	default:
		event.Status = "[redacted]"
	}
	switch event.FailureCode {
	case "", store.FailureCodeInvalidInput, store.FailureCodeAgent, store.FailureCodeCommand, store.FailureCodeInvalidOutput, store.FailureCodeValidation, store.FailureCodeInternal, store.FailureCodeWorkspace, store.FailureCodeInvocationLimit:
	default:
		event.FailureCode = "[redacted]"
	}
	if event.ProviderKind != "" && (store.ProviderFailure{Kind: event.ProviderKind, Attempts: 1}).Validate() != nil {
		event.ProviderKind = "[redacted]"
	}
	return event
}
