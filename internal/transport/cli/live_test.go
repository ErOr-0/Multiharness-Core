package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func progressFixture(tty bool, width int) (*progressSink, *bytes.Buffer) {
	buffer := &bytes.Buffer{}
	p := newPresentation(io.Discard, buffer).progress
	p.view.size = func() (int, bool) { return width, tty }
	return p, buffer
}

func TestProgressModesRespectTerminalEnvironmentAndJSON(t *testing.T) {
	for _, test := range []struct {
		name, format, color, progress           string
		tty, quiet, friendly, colored, animated bool
		env                                     map[string]string
	}{
		{name: "terminal", tty: true, friendly: true, colored: true, animated: true},
		{name: "pipe"},
		{name: "plain", progress: "plain", friendly: true},
		{name: "forced colour pipe", color: "always", friendly: true, colored: true},
		{name: "never colour", tty: true, color: "never", friendly: true, animated: true},
		{name: "NO_COLOR", tty: true, color: "always", env: map[string]string{"NO_COLOR": "1"}, friendly: true, animated: true},
		{name: "dumb", tty: true, env: map[string]string{"TERM": "dumb"}, friendly: true},
		{name: "CI", tty: true, env: map[string]string{"CI": "true"}, friendly: true},
		{name: "JSON", tty: true, format: "json", color: "always", progress: "plain"},
		{name: "quiet", tty: true, quiet: true, friendly: true, colored: true, animated: true},
		{name: "off", tty: true, progress: "off", friendly: true, colored: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, buffer := progressFixture(test.tty, 80)
			cfg := config.Defaults()
			if test.format != "" {
				cfg.LogFormat = test.format
			}
			if test.color != "" {
				cfg.Color = test.color
			}
			if test.progress != "" {
				cfg.Progress = test.progress
			}
			p.format, p.quiet = cfg.LogFormat, test.quiet
			p.configure(cfg, func(key string) (string, bool) { value, ok := test.env[key]; return value, ok })
			if p.view.friendly != test.friendly || p.view.color != test.colored || p.view.animate != test.animated {
				t.Fatalf("unexpected mode: %+v", p.view)
			}
			p.Publish(workflow.Event{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStagePlanning})
			p.AgentActivity(activity.Event{Agent: activity.Codex, Kind: activity.CommandRunning})
			p.tick(time.Now())
			if test.quiet || test.progress == "off" {
				if buffer.Len() != 0 {
					t.Fatal("quiet/off output")
				}
				return
			}
			if cfg.LogFormat == "json" {
				for _, line := range bytes.Split(bytes.TrimSpace(buffer.Bytes()), []byte{'\n'}) {
					if !json.Valid(line) {
						t.Fatal("invalid JSONL")
					}
				}
				if strings.ContainsAny(buffer.String(), "\x1b\r") {
					t.Fatal("terminal escape in JSON")
				}
			} else if !test.colored && !test.animated && strings.ContainsAny(buffer.String(), "\x1b\r") {
				t.Fatal("escape in plain output")
			}
		})
	}
}

func TestLiveElapsedActivityRetrySwitchResizeAndPromptPause(t *testing.T) {
	p, buffer := progressFixture(true, 160)
	p.configure(config.Defaults(), nil)
	p.view.color = false
	p.Publish(workflow.Event{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStagePlanning})
	start := p.view.stageStarted
	p.tick(start.Add(5 * time.Second))
	if !strings.Contains(buffer.String(), "elapsed 5s | waiting for activity") {
		t.Fatal("idle timer implied agent progress")
	}
	p.AgentActivity(activity.Event{Agent: activity.Codex, Kind: activity.CommandRunning})
	p.tick(start.Add(6 * time.Second))
	p.tick(start.Add(10 * time.Second))
	if !strings.Contains(buffer.String(), "last update 4s ago: command running") {
		t.Fatal("last activity age lost")
	}
	p.Publish(workflow.Event{Type: workflow.EventTypeAgentRetryScheduled, Stage: store.WorkflowStagePlanning, RetryAttempt: 1, RetryDelayMillis: 5000, ProviderKind: store.ProviderRateLimited})
	p.tick(p.view.retryUntil.Add(-2 * time.Second))
	if !strings.Contains(buffer.String(), "retry in 2s") {
		t.Fatal("missing retry countdown")
	}
	// Coalescing may skip Starting; any new call activity must clear the countdown.
	p.AgentActivity(activity.Event{Agent: activity.Codex, Kind: activity.TurnStarted})
	p.tick(time.Now())
	if !p.view.retryUntil.IsZero() {
		t.Fatal("retry display outlived wait")
	}
	resume, err := p.PauseProgress()
	if err != nil {
		t.Fatal(err)
	}
	before := buffer.String()
	p.AgentActivity(activity.Event{Agent: activity.Codex, Kind: activity.ResponseReceived})
	p.tick(time.Now())
	if buffer.String() != before {
		t.Fatal("live output overwrote consent prompt")
	}
	resume()
	p.Publish(workflow.Event{Type: workflow.EventTypeAgentSwitched, Stage: store.WorkflowStagePlanning})
	if p.stageLabel(store.WorkflowStagePlanning) != "OpenCode planning" {
		t.Fatal("fallback label wrong")
	}
	p.Publish(workflow.Event{Type: workflow.EventTypeAgentSwitched, Stage: store.WorkflowStageRepair})
	if p.stageLabel(store.WorkflowStageImplementation) != "Codex implementing" || p.stageLabel(store.WorkflowStageRepair) != "Codex repairing" {
		t.Fatal("implementation and repair labels diverged")
	}
	p.view.size = func() (int, bool) { return 20, true }
	buffer.Reset()
	p.tick(time.Now())
	line := strings.TrimPrefix(buffer.String(), "\r\x1b[2K")
	if len(line) > 19 || strings.Contains(line, "\n") {
		t.Fatal("live line wraps on narrow terminal")
	}
	p.Publish(workflow.Event{Type: workflow.EventTypeStageCompleted, Stage: store.WorkflowStagePlanning})
	before = buffer.String()
	p.tick(time.Now())
	if buffer.String() != before || p.view.lineVisible {
		t.Fatal("stage completion left a live line")
	}
}

func TestActivityCoalescingNeverWaitsForPresentationLock(t *testing.T) {
	p, _ := progressFixture(false, 80)
	p.configure(config.Defaults(), nil)
	p.mu.Lock() // Simulate blocked terminal I/O while providers continue streaming.
	completed := make(chan struct{})
	go func() {
		defer close(completed)
		for range 10000 {
			p.AgentActivity(activity.Event{Agent: activity.Codex, Kind: activity.CommandRunning})
		}
		p.AgentActivity(activity.Event{Agent: activity.OpenCode, Kind: activity.ResponseReceived})
	}()
	select {
	case <-completed:
	case <-time.After(time.Second):
		p.mu.Unlock()
		t.Fatal("agent output blocked on presentation")
	}
	p.mu.Unlock()
	if len(p.pending) != 1 || cap(p.pending) != 1 {
		t.Fatal("unbounded activity buffer")
	}
	if got := <-p.pending; got.Agent != activity.OpenCode || got.Kind != activity.ResponseReceived {
		t.Fatal("latest activity lost")
	}
	p.AgentActivity(activity.Event{Agent: activity.Codex, Kind: "SECRET\x1b[2J"})
	if len(p.pending) != 0 {
		t.Fatal("unsafe activity accepted")
	}
}

func TestProgressWorkerStopsOnCancellationAndOutputFailure(t *testing.T) {
	for _, outputFailure := range []bool{false, true} {
		p, _ := progressFixture(true, 80)
		p.configure(config.Defaults(), nil)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		p.cancel = cancel
		if outputFailure {
			p.writer = brokenProgressWriter{}
		}
		p.Publish(workflow.Event{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStagePlanning})
		p.start(ctx)
		if !outputFailure {
			cancel()
		}
		select {
		case <-p.done:
		case <-time.After(time.Second):
			t.Fatal("progress worker leaked")
		}
		p.stop()
		p.stop()
		if !p.view.stopped || p.view.lineVisible {
			t.Fatal("terminal did not stop cleanly")
		}
		if outputFailure && ctx.Err() == nil {
			t.Fatal("writer failure did not cancel workflow")
		}
	}
}

type brokenProgressWriter struct{}

func (brokenProgressWriter) Write(data []byte) (int, error) {
	return 0, errors.New("SECRET writer failure")
}

type approvalFunc func(context.Context, store.AgentSwitch) (bool, error)

func (f approvalFunc) ConfirmFallback(ctx context.Context, choice store.AgentSwitch) (bool, error) {
	return f(ctx, choice)
}

func TestBillingDecoratorPausesAndResumesEvenOnCancellation(t *testing.T) {
	p, buffer := progressFixture(true, 80)
	p.configure(config.Defaults(), nil)
	p.Publish(workflow.Event{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageImplementation})
	p.tick(time.Now())
	choice := store.AgentSwitch{Stage: store.WorkflowStageImplementation, From: "OpenCode", To: "Codex", Model: "fixture", CanWrite: true}
	wrapped := WithProgressApproval(approvalFunc(func(context.Context, store.AgentSwitch) (bool, error) {
		if !p.view.paused || p.view.lineVisible {
			t.Fatal("prompt not paused")
		}
		before := buffer.String()
		p.tick(time.Now())
		if buffer.String() != before {
			t.Fatal("prompt overwritten")
		}
		return false, context.Canceled
	}), p)
	if yes, err := wrapped.ConfirmFallback(t.Context(), choice); yes || !errors.Is(err, context.Canceled) || p.view.paused {
		t.Fatal("consent/cancellation/resume changed")
	}
	p.err = errors.New("SECRET")
	called := false
	wrapped = WithProgressApproval(approvalFunc(func(context.Context, store.AgentSwitch) (bool, error) { called = true; return true, nil }), p)
	if yes, err := wrapped.ConfirmFallback(t.Context(), choice); yes || err == nil || strings.Contains(err.Error(), "SECRET") || called {
		t.Fatal("failed output authorized fallback or leaked")
	}
}

func TestHumanSummaryDistinguishesTerminalOutcomesWithoutFreeText(t *testing.T) {
	for _, status := range []store.TaskStatus{store.TaskStatusAnswered, store.TaskStatusApproved, store.TaskStatusFailed, store.TaskStatusCancelled, store.TaskStatusRepairLimitReached} {
		p, buffer := progressFixture(false, 80)
		cfg := config.Defaults()
		cfg.Progress = "plain"
		p.configure(cfg, nil)
		output := store.TaskOutput{Status: status, Summary: "SECRET", AgentInvocations: 3, RepairAttempts: 1, Validation: &store.ValidationReport{Checks: []store.ValidationEvidence{{Passed: true}, {Passed: false}}}}
		if status == store.TaskStatusFailed {
			output.Failure = &store.TaskFailure{Stage: store.WorkflowStagePlanning, Code: store.FailureCodeAgent, Message: "SECRET", Provider: &store.ProviderFailure{Kind: store.ProviderBillingExhausted, Attempts: 1}}
		}
		p.result(output, exitCode(status))
		if strings.Contains(buffer.String(), "SECRET") || !strings.Contains(buffer.String(), "Latest validation: 1 passed, 1 failed") {
			t.Fatal("untrusted or missing summary")
		}
		if status == store.TaskStatusRepairLimitReached && (!strings.Contains(buffer.String(), "not approved") || strings.Contains(buffer.String(), "[OK]")) {
			t.Fatal("repair limit appeared successful")
		}
		if status == store.TaskStatusFailed && !strings.Contains(buffer.String(), "billing_exhausted") {
			t.Fatal("missing safe failure category")
		}
	}
}

func TestHumanViewReportsFinalDeliveryFailureWithoutWriterDiagnostics(t *testing.T) {
	var stderr bytes.Buffer
	p := newPresentation(brokenProgressWriter{}, &stderr)
	cfg := config.Defaults()
	cfg.Progress = "plain"
	p.progress.configure(cfg, nil)
	output := store.TaskOutput{Status: store.TaskStatusAnswered, Summary: "fixture answer", Plan: &store.Plan{Action: store.PlanActionAnswer, Summary: "fixture", Answer: "fixture answer"}}
	if code := p.finish(output, ExitSuccess); code != ExitFailed {
		t.Fatal("stdout failure appeared successful")
	}
	if strings.Contains(stderr.String(), "SECRET") || !strings.Contains(stderr.String(), "[FAIL] Result could not be written; process exit 1.") {
		t.Fatal("missing or unsafe delivery failure")
	}
}
