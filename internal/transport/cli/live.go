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
// The optional failure detail view uses an alternate screen. Its controller
// restores terminal input on close, cancellation and before consent prompts.
type liveView struct {
	workspaceScan                                 string
	trueColor                                     bool
	routingSource                                 store.DecisionSource
	size                                          func() (int, bool)
	friendly, color, animate, expanded            bool
	sectionShown                                  bool
	active, paused, stopped, lineVisible          bool
	modal                                         bool
	pager                                         *failurePager
	started, stageStarted, lastUpdate, retryUntil time.Time
	last                                          activity.Event
	frame                                         int
	repairAttempt                                 int
	switched                                      map[store.WorkflowStage]bool
	summary                                       string
	plannerHarness                                string
	implementerHarness                            string
	reviewerHarness                               string
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
	p.view.color = p.view.friendly && terminalColors(cfg.Color, tty, lookup)
	p.view.trueColor = terminalTrueColor(lookup)
	p.view.expanded = cfg.Progress == "expanded"
	p.view.animate = p.view.friendly && tty && (cfg.Progress == "auto" || p.view.expanded) && env("TERM") != "dumb" && env("CI") == ""
	p.view.started = time.Now()
	p.view.plannerHarness = cfg.Planner.Harness
	p.view.implementerHarness = cfg.Implementer.Harness
	p.view.reviewerHarness = cfg.Reviewer.Harness
	p.view.switched = make(map[store.WorkflowStage]bool)
}

func (p *progressSink) start(ctx context.Context) {
	if p.quiet {
		return
	}
	p.stopCh, p.done = make(chan struct{}), make(chan struct{})
	p.runCtx = ctx
	go func() {
		defer close(p.done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		defer func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.flushFailures()
			p.flushTranscript()
			p.closeFailureModal()
			p.clearLine()
			p.view.stopped = true
		}()
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
	if p.control != nil && p.view.animate && !p.view.expanded && p.format == "text" {
		p.control.startProgress(ctx, p)
	}
}

func (p *progressSink) tick(now time.Time) {
	p.mu.Lock()
	p.flushFailures()
	p.flushActivity(now)
	p.drawLive(now)
	if p.view.modal && p.view.pager != nil {
		w, h, _ := terminalDimensions(p.writer)
		if w > 0 && h > 0 && (w != p.view.pager.width || h != p.view.pager.height) {
			p.drawFailurePage()
		}
	}
	p.mu.Unlock()
	if p.control != nil && p.runCtx != nil && p.view.animate && !p.view.expanded && p.format == "text" {
		p.control.startProgress(p.runCtx, p)
	}
}

func (p *progressSink) stop() {
	p.mu.Lock()
	p.view.stopped = true
	p.mu.Unlock()
	if p.control != nil {
		p.control.stopProgress()
	}
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
	if event.Text != "" && p.view.expanded && !p.quiet && p.format == "text" {
		event.Text = activity.DisplayText(event.Text)
		preview := event
		preview.Command, preview.Error, preview.Output, preview.Detailed = "", "", "", false
		select {
		case p.transcript <- preview:
		default:
			p.omitted.Add(1)
		}
	}
	if event.Kind == activity.ToolFailed {
		event.Stage = ""
		if stage, ok := p.activityStage.Load().(store.WorkflowStage); ok {
			event.Stage = string(stage)
		}
		event.Text = activity.DisplayText(event.Text)
		event.Summary = activity.DisplayText(event.Summary)
		event.Command = activity.DetailText(event.Command)
		event.Error = activity.DetailText(event.Error)
		event.Output = activity.DetailText(event.Output)
		p.failureCount.Add(1)
		select {
		case p.failureInbox <- event:
		default:
			select {
			case <-p.failureInbox:
			default:
			}
			select {
			case p.failureInbox <- event:
			default:
			}
		}
	}
	event.Text = ""
	event.Summary = ""
	event.Command, event.Error, event.Output, event.Detailed = "", "", "", false
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

// The failure inbox survives latest-wins progress coalescing, so a later agent
// update cannot erase the reason the user needs to inspect.
func (p *progressSink) flushFailures() {
	if p.view.paused {
		return
	}
	for {
		select {
		case event := <-p.failureInbox:
			if event.Summary == "" {
				event.Summary = "tool failed"
			}
			if event.Text == "" {
				event.Text = "The provider did not include a reason for this failure."
			}
			if n := len(p.failures); n > 0 && p.failures[n-1] == event {
				continue
			}
			if len(p.failures) == 8 {
				p.failures = p.failures[1:]
			}
			p.failures = append(p.failures, event)
			if !p.quiet && p.format == "text" && !p.view.expanded && p.view.friendly && !p.view.modal {
				p.clearLine()
				view := p.terminalView()
				label := string(event.Agent)
				if event.Stage != "" {
					label += " · " + event.Stage
				}
				message := fmt.Sprintf("%s: %s · click ▶ or press d for details", label, event.Summary)
				p.writeBytes([]byte(view.styledText(view.paragraph("! "+message, 2, "33"))))
			}
		default:
			return
		}
	}
}

func (p *progressSink) flushActivity(now time.Time) {
	if p.view.paused || p.view.modal {
		return
	}
	p.flushTranscript()
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

// A separate bounded queue preserves output order without blocking process
// readers on a slow terminal. Overflow is explicit, never silently hidden.
func (p *progressSink) flushTranscript() {
	defer func() {
		if omitted := p.omitted.Swap(0); omitted > 0 && !p.quiet && p.format == "text" {
			p.clearLine()
			p.writeBytes([]byte(fmt.Sprintf("[output backlog: %d events omitted]\n", omitted)))
		}
	}()
	for range 128 {
		select {
		case event := <-p.transcript:
			if !p.quiet && p.format == "text" {
				p.clearLine()
				p.writeBytes([]byte(fmt.Sprintf("\n[%s · %s]\n%s\n", event.Agent, activityLabel(event.Kind), event.Text)))
			}
		default:
			return
		}
	}
}

func (p *progressSink) beforeEvent(event workflow.Event, now time.Time) {
	p.flushActivity(now)
	p.clearLine()
	switch event.Type {
	case workflow.EventTypeRoutingDecided:
		p.view.routingSource = event.DecisionSource
	case workflow.EventTypeStageStarted:
		p.view.active, p.view.stageStarted = true, now
		p.view.repairAttempt = event.RepairAttempt
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
	p.flushFailures()
	p.flushActivity(time.Now())
	p.clearLine()
	p.view.paused = true
	p.mu.Unlock()
	if p.control != nil {
		p.control.stopProgress()
	}
	p.mu.Lock()
	p.closeFailureModal()
	err := p.err
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		p.view.paused = false
		p.mu.Unlock()
		if p.control != nil && p.runCtx != nil && p.runCtx.Err() == nil && p.view.animate && !p.view.expanded && p.format == "text" {
			p.control.startProgress(p.runCtx, p)
		}
	}, err
}

// toggleFailureModal is called only for an explicit terminal click or key.
// The workflow keeps running; normal progress resumes when the view closes.
func (p *progressSink) toggleFailureModal() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.view.paused || p.view.stopped || p.failureCount.Load() == 0 || p.err != nil {
		return
	}
	if p.view.modal {
		p.closeFailureModal()
		p.drawLive(time.Now())
		return
	}
	p.flushFailures()
	p.clearLine()
	p.writeBytes([]byte("\x1b[?1049h"))
	if p.err != nil {
		return
	}
	p.view.modal = true
	p.view.pager = newFailurePager(p.failures, p.failureCount.Load())
	p.drawFailurePage()
}

// failurePageInput returns whether the expanded view consumed this key.
func (p *progressSink) failurePageInput(key string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.view.modal || p.view.pager == nil {
		return false
	}
	if p.view.pager.input(key) {
		p.closeFailureModal()
		p.drawLive(time.Now())
	} else {
		p.drawFailurePage()
	}
	return true
}

func (p *progressSink) drawFailurePage() {
	w, h, _ := terminalDimensions(p.writer)
	p.writeBytes([]byte(p.view.pager.render(p.terminalView(), w, h)))
}

// closeFailureModal requires p.mu and is safe during cancellation and prompts.
func (p *progressSink) closeFailureModal() {
	if p.view.modal {
		p.view.modal = false
		p.writeBytes([]byte("\x1b[?1049l"))
	}
}

func (p *progressSink) clearLine() {
	if p.view.lineVisible {
		p.writeBytes([]byte("\r\x1b[2K"))
		p.view.lineVisible = false
	}
}

func (p *progressSink) drawLive(now time.Time) {
	if !p.view.animate || !p.view.active || p.view.paused || p.view.modal || p.view.stopped || p.quiet || p.err != nil {
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
	frames := []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
	marker := " "
	if p.failureCount.Load() > 0 {
		marker = "▶"
	}
	label := fmt.Sprintf("  %s %c %s · %s", marker, frames[p.view.frame%len(frames)], p.stageLabel(p.stage), elapsed(now.Sub(p.view.stageStarted)))
	if p.view.repairAttempt > 0 {
		label += fmt.Sprintf(" · repair %d", p.view.repairAttempt)
	}
	p.view.frame++
	if p.view.workspaceScan != "" {
		label += " | " + p.view.workspaceScan
	} else if !p.view.retryUntil.IsZero() {
		if remaining := p.view.retryUntil.Sub(now); remaining > 0 {
			label += " | retry in " + elapsed(remaining+time.Second-time.Nanosecond)
		} else {
			label += " | retry wait complete; preparing next attempt"
		}
	} else if p.view.lastUpdate.IsZero() {
		label += " | waiting for activity"
	} else {
		latest := activityLabel(p.view.last.Kind)
		if p.view.last.Kind == activity.ToolFailed && len(p.failures) > 0 {
			latest = p.failures[len(p.failures)-1].Summary
		}
		label += fmt.Sprintf(" | last update %s ago: %s", elapsed(now.Sub(p.view.lastUpdate)), latest)
	}
	if n := p.failureCount.Load(); n > 0 {
		label += fmt.Sprintf(" | !%d", n)
	}
	// Labels use single-cell runes. Leave the last column unused to avoid soft wraps;
	// query width every frame so resize does not require global signal handlers.
	if runes := []rune(label); len(runes) >= width {
		label = string(runes[:width-1])
	}
	p.writeBytes([]byte("\r\x1b[2K" + p.paint(label, "36")))
	p.view.lineVisible = true
}

func elapsed(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	return (duration / time.Second * time.Second).String()
}
