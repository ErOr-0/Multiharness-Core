package cli

import (
	"context"
	"fmt"
	"time"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// liveView is invocation-local presentation state, protected by progressSink.mu.
// No terminal screen mode or hidden cursor is used, so interruption cannot leave
// the user's terminal in raw/alternate-screen mode.
type liveView struct {
	size                                          func() (int, bool)
	friendly, color, animate                      bool
	active, paused, stopped, lineVisible          bool
	started, stageStarted, lastUpdate, retryUntil time.Time
	last                                          activity.Event
	frame                                         int
	switched                                      map[store.WorkflowStage]bool
	summary                                       string
	plannerHarness                                string
}

func (p *progressSink) configure(cfg config.Config, lookup func(string) (string, bool)) {
	env := func(key string) string {
		if lookup != nil {
			value, _ := lookup(key)
			return value
		}
		return ""
	}
	if p.view.size == nil {
		p.view.size = func() (int, bool) { return terminalSize(p.writer) }
	}
	_, tty := p.view.size()
	p.quiet = p.quiet || cfg.Progress == "off"
	p.view.friendly = cfg.LogFormat == "text" && (tty || cfg.Progress == "plain" || cfg.Color == "always")
	p.view.color = p.view.friendly && cfg.Color != "never" && (tty || cfg.Color == "always") && env("NO_COLOR") == "" && env("TERM") != "dumb" && (env("CI") == "" || cfg.Color == "always")
	p.view.animate = p.view.friendly && tty && cfg.Progress == "auto" && env("TERM") != "dumb" && env("CI") == ""
	p.view.started = time.Now()
	p.view.plannerHarness = cfg.PlannerHarness
	p.view.switched = make(map[store.WorkflowStage]bool)
}

func (p *progressSink) start(ctx context.Context) {
	if p.quiet {
		return
	}
	p.stopCh, p.done = make(chan struct{}), make(chan struct{})
	go func() {
		defer close(p.done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		defer func() { p.mu.Lock(); defer p.mu.Unlock(); p.clearLine(); p.view.stopped = true }()
		for {
			select {
			case <-ctx.Done():
				return
			case <-p.stopCh:
				return
			case now := <-ticker.C:
				p.tick(now)
			}
		}
	}()
}

func (p *progressSink) tick(now time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.flushActivity(now)
	p.drawLive(now)
}

func (p *progressSink) stop() {
	if p.stopCh != nil {
		p.stopOnce.Do(func() { close(p.stopCh) })
		<-p.done
	}
}

// AgentActivity runs on a child-process output reader. A one-slot, latest-wins
// mailbox never waits for terminal I/O and cannot grow with provider traffic.
func (p *progressSink) AgentActivity(event activity.Event) {
	if !event.Valid() || p.pending == nil {
		return
	}
	select {
	case p.pending <- event:
	default:
		select {
		case <-p.pending:
		default:
		}
		select {
		case p.pending <- event:
		default:
		}
	}
}

func (p *progressSink) flushActivity(now time.Time) {
	if p.view.paused {
		return
	}
	select {
	case event := <-p.pending:
		if !p.view.active {
			return
		}
		previous := p.view.last
		p.view.last, p.view.lastUpdate = event, now
		p.view.retryUntil = time.Time{} // A new call can outpace the display's starting notice.
		if previous != event {
			p.write(logRecord{Level: "info", Code: "agent_activity", Agent: event.Agent, Activity: event.Kind, Event: workflow.Event{Stage: p.stage}})
		}
	default:
	}
}

func (p *progressSink) beforeEvent(event workflow.Event, now time.Time) {
	p.flushActivity(now)
	p.clearLine()
	switch event.Type {
	case workflow.EventTypeStageStarted:
		p.view.active, p.view.stageStarted = true, now
		p.view.last, p.view.lastUpdate, p.view.retryUntil = activity.Event{}, time.Time{}, time.Time{}
	case workflow.EventTypeStageCompleted, workflow.EventTypeStageFailed, workflow.EventTypeWorkflowCompleted:
		p.view.active = false
	case workflow.EventTypeAgentRetryScheduled:
		p.view.retryUntil = now.Add(time.Duration(event.RetryDelayMillis) * time.Millisecond)
		p.view.last, p.view.lastUpdate = activity.Event{}, time.Time{}
	case workflow.EventTypeAgentSwitched:
		stage := event.Stage
		if stage == store.WorkflowStageRepair {
			stage = store.WorkflowStageImplementation
		}
		if p.view.switched != nil {
			p.view.switched[stage] = true
		}
		p.view.last, p.view.lastUpdate = activity.Event{}, time.Time{}
	}
}

// PauseProgress is consumed by the CLI billing-approval decorator, not the core
// workflow. Timer and queued updates cannot overwrite a human consent prompt.
func (p *progressSink) PauseProgress() (func(), error) {
	p.mu.Lock()
	p.flushActivity(time.Now())
	p.clearLine()
	p.view.paused = true
	err := p.err
	p.mu.Unlock()
	return func() { p.mu.Lock(); p.view.paused = false; p.mu.Unlock() }, err
}

func (p *progressSink) clearLine() {
	if p.view.lineVisible {
		p.writeBytes([]byte("\r\x1b[2K"))
		p.view.lineVisible = false
	}
}

func (p *progressSink) drawLive(now time.Time) {
	if !p.view.animate || !p.view.active || p.view.paused || p.view.stopped || p.quiet || p.err != nil {
		return
	}
	width, tty := p.view.size()
	if !tty {
		p.clearLine()
		return
	}
	if width < 2 {
		width = 80
	}
	label := fmt.Sprintf("[%c] %s | elapsed %s", "|/-\\"[p.view.frame%4], p.stageLabel(p.stage), elapsed(now.Sub(p.view.stageStarted)))
	p.view.frame++
	if !p.view.retryUntil.IsZero() {
		if remaining := p.view.retryUntil.Sub(now); remaining > 0 {
			label += " | retry in " + elapsed(remaining+time.Second-time.Nanosecond)
		} else {
			label += " | retry wait complete; preparing next attempt"
		}
	} else if p.view.lastUpdate.IsZero() {
		label += " | waiting for activity"
	} else {
		label += fmt.Sprintf(" | last update %s ago: %s", elapsed(now.Sub(p.view.lastUpdate)), activityLabel(p.view.last.Kind))
	}
	// Labels are fixed ASCII. Leave the last column unused to avoid soft wraps;
	// query width every frame so resize does not require global signal handlers.
	if len(label) >= width {
		label = label[:width-1]
	}
	p.writeBytes([]byte("\r\x1b[2K" + p.paint(label, "34")))
	p.view.lineVisible = true
}

func elapsed(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	return (duration / time.Second * time.Second).String()
}
