package progress

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/contract"
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

// Sink publishes one run's progress to a writer. Format, Quiet, NoChecks,
// Cancel and Control are set by the caller before Start; everything else is
// guarded by the sink. Cancel is called when the writer fails so a run does
// not continue unobserved.
type Sink struct {
	mu                    sync.Mutex
	writer                io.Writer
	Format, taskID, runID string
	Quiet, NoChecks       bool
	Cancel                context.CancelFunc
	err                   error
	stage                 contract.WorkflowStage
	activityStage         atomic.Value
	view                  liveView
	pending               chan activity.Event
	transcript            chan activity.Event
	failureInbox          chan activity.Event
	failures              []activity.Event
	failureCount          atomic.Uint64
	omitted               atomic.Uint64
	stopOnce              sync.Once
	stopCh, done          chan struct{}
	Control               Control
	runCtx                context.Context
}

// New starts a text-format sink for one task run, writing to writer.
func New(writer io.Writer) *Sink {
	return &Sink{
		writer: writer, Format: "text", taskID: "task_" + rand.Text(), runID: "run_" + rand.Text(),
		pending:      make(chan activity.Event, 1),
		transcript:   make(chan activity.Event, 128),
		failureInbox: make(chan activity.Event, 8),
	}
}

// Control lets an interactive terminal read keys while the live display is
// active, to open the failure view. Scripted runs and other transports leave
// it nil and retain their existing behavior.
type Control interface {
	StartProgress(context.Context, *Sink)
	StopProgress()
}

func (p *Sink) Publish(event workflow.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	event = redactEvent(event)
	p.beforeEvent(event, time.Now())
	p.stage = event.Stage
	p.activityStage.Store(event.Stage)
	level := "info"
	if event.Type == workflow.EventTypeStageFailed {
		level = "error"
	}
	p.write(logRecord{Level: level, Event: event})
	if p.NoChecks && !event.AuthorizedValidation && event.Type == workflow.EventTypeStageStarted && event.Stage == contract.WorkflowStageValidation {
		p.write(logRecord{Level: "warning", Code: "no_validation_checks", Event: workflow.Event{Stage: event.Stage, Sequence: event.Sequence}})
	}
}

func (p *Sink) Result(output contract.TaskOutput, code int) {
	p.Stop()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.view.summary = resultSummary(output)
	p.write(logRecord{Level: "info", Code: "result_ready", ExitCode: &code, Event: redactEvent(workflow.Event{Status: output.Status})})
}

// FailureDetails is used only by the interactive terminal's on-demand view.
// It is deliberately separate from structured logs and limited to recent items.
func (p *Sink) FailureDetails() ([]activity.Event, uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.flushFailures()
	return append([]activity.Event(nil), p.failures...), p.failureCount.Load()
}

// AcceptsFailureKeys reports whether a key reader may open the failure view:
// something failed and the live display is neither paused nor finished.
func (p *Sink) AcceptsFailureKeys() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.failureCount.Load() > 0 && !p.view.paused && !p.view.stopped
}

// ExclusiveOutput runs write while no progress frame can interleave with it.
func (p *Sink) ExclusiveOutput(write func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	write()
}

// FailureViewOpen reports whether the full-screen failure view is showing.
func (p *Sink) FailureViewOpen() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.view.modal
}

// SetTerminalSize replaces terminal detection, for output that is not a file.
func (p *Sink) SetTerminalSize(size func() (int, bool)) { p.view.size = size }

// Correlation returns the task and run identifiers carried by every record.
func (p *Sink) Correlation() (taskID, runID string) { return p.taskID, p.runID }

func (p *Sink) ResultDeliveryFailed(code int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.write(logRecord{Level: "error", Code: "result_output_failed", ExitCode: &code})
}

// A runtime-selection notice has no workflow sequence number and cannot change
// the workflow's stage. Only numeric release metadata reaches the log.
func (p *Sink) CodexRuntimeSelected(version string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(version) > 32 || len(strings.Split(version, ".")) != 3 || strings.Trim(version, "0123456789.") != "" {
		version = "[redacted]"
	}
	p.write(logRecord{Level: "info", Code: "codex_runtime_selected", RuntimeVersion: version})
	return p.err
}

// write holds mu, keeping each JSONL record atomic for this run's publishers.
func (p *Sink) write(record logRecord) {
	if p.Quiet || p.err != nil {
		return
	}
	record.Version, record.Time = 1, time.Now().UTC().Format(time.RFC3339Nano)
	record.TaskID, record.RunID = p.taskID, p.runID
	var data []byte
	if p.Format == "json" {
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
		if record.Type == workflow.EventTypeRoutingDecided {
			fmt.Fprintf(&line, " route=%s source=%s confidence=%.2f", record.Route, record.DecisionSource, record.Confidence)
			if record.RoutingFallback != "" {
				fmt.Fprintf(&line, " fallback=%s", record.RoutingFallback)
			}
		}
		if record.RepairAttempt > 0 {
			fmt.Fprintf(&line, " attempt=%d", record.RepairAttempt)
		}
		if record.RetryAttempt > 0 {
			fmt.Fprintf(
				&line,
				" retry_attempt=%d provider_kind=%s agent_invocations=%d",
				record.RetryAttempt,
				record.ProviderKind,
				record.AgentInvocations,
			)
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
func (p *Sink) writeBytes(data []byte) {
	if p.Quiet || p.err != nil {
		return
	}
	var n int
	n, p.err = p.writer.Write(data)
	if n != len(data) && p.err == nil {
		p.err = io.ErrShortWrite
	}
	if p.err != nil && p.Cancel != nil {
		p.Cancel()
	}
}

func (p *Sink) Failure() (contract.WorkflowStage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	stage := p.stage
	if stage == "" || stage == "[redacted]" {
		stage = contract.WorkflowStageIntake
	}
	return stage, p.err
}

func redactEvent(event workflow.Event) workflow.Event {
	if event.Route != "" && !event.Route.Valid() {
		event.Route = "[redacted]"
	}
	if event.DecisionSource != "" && event.DecisionSource != contract.DecisionJev && event.DecisionSource != contract.DecisionFallback {
		event.DecisionSource = "[redacted]"
	}
	if event.RoutingFallback != "" && !event.RoutingFallback.Valid() {
		event.RoutingFallback = "[redacted]"
	}
	if math.IsNaN(event.Confidence) || math.IsInf(event.Confidence, 0) || event.Confidence < 0 || event.Confidence > 1 {
		event.Confidence = 0
	}
	if event.RetryDelayMillis < 0 || event.RetryDelayMillis > int64(24*time.Hour/time.Millisecond) {
		event.RetryDelayMillis = 0
	}
	switch event.Type {
	case "", workflow.EventTypeStageStarted, workflow.EventTypeStageProgress, workflow.EventTypeStageCompleted, workflow.EventTypeStageFailed, workflow.EventTypeWorkflowCompleted, workflow.EventTypeAgentRetryScheduled, workflow.EventTypeRoutingDecided, workflow.EventTypeWorkspaceRetry:
	default:
		event.Type = "[redacted]"
	}
	switch event.Stage {
	case "", contract.WorkflowStageRouting, contract.WorkflowStageAnswering, contract.WorkflowStageDelegation, contract.WorkflowStageIntake, contract.WorkflowStagePlanning, contract.WorkflowStageImplementation, contract.WorkflowStageValidation, contract.WorkflowStageReview, contract.WorkflowStageRepair:
	default:
		event.Stage = "[redacted]"
	}
	switch event.Status {
	case "", contract.TaskStatusResponded, contract.TaskStatusNeedsInput, contract.TaskStatusTimedOut, contract.TaskStatusAnswered, contract.TaskStatusApproved, contract.TaskStatusFailed, contract.TaskStatusCancelled, contract.TaskStatusRepairLimitReached:
	default:
		event.Status = "[redacted]"
	}
	switch event.FailureCode {
	case "", contract.FailureCodeInvalidInput, contract.FailureCodeAgent, contract.FailureCodePermission, contract.FailureCodeCommand, contract.FailureCodeInvalidOutput, contract.FailureCodeValidation, contract.FailureCodeValidationInput, contract.FailureCodeInternal, contract.FailureCodeWorkspace, contract.FailureCodeInvocationLimit:
	default:
		event.FailureCode = "[redacted]"
	}
	if event.ProviderKind != "" && (contract.ProviderFailure{Kind: event.ProviderKind, Attempts: 1}).Validate() != nil {
		event.ProviderKind = "[redacted]"
	}
	return event
}
