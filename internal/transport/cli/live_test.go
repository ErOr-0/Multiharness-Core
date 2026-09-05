package cli

import (
	"bytes"
	"context"
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

func TestPlanningProgressUsesSelectedHarnessAndConfirmedFallback(t *testing.T) {
	p, buffer := progressFixture(false, 120)
	cfg := config.Defaults()
	cfg.PlannerHarness, cfg.Progress = "opencode", "plain"
	p.configure(cfg, nil)
	p.Publish(workflow.Event{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStagePlanning})
	if !strings.Contains(buffer.String(), "OpenCode planning") {
		t.Fatal("planning progress named the wrong harness")
	}
	p.Publish(workflow.Event{Type: workflow.EventTypeAgentSwitched, Stage: store.WorkflowStagePlanning})
	if !strings.Contains(buffer.String(), "Confirmed provider switch: Codex planning") {
		t.Fatal("planning fallback named the wrong harness")
	}
	if p.stageLabel(store.WorkflowStageReview) != "Codex reviewing" || p.stageLabel(store.WorkflowStageRepair) != "OpenCode repairing" {
		t.Fatal("planning selection altered another role's label")
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
