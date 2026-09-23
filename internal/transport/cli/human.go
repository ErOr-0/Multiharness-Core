package cli

import (
	"fmt"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func (p *progressSink) paint(text, color string) string {
	if !p.view.color {
		return text
	}
	return themePaint(text, color, p.view.trueColor)
}

func (p *progressSink) stageLabel(stage store.WorkflowStage) string {
	role := stage
	if role == store.WorkflowStageRepair {
		role = store.WorkflowStageImplementation
	}
	harness := "codex"
	switch role {
	case store.WorkflowStagePlanning, store.WorkflowStageAnswering:
		harness = p.view.plannerHarness
	case store.WorkflowStageImplementation, store.WorkflowStageDelegation:
		harness = p.view.implementerHarness
	case store.WorkflowStageReview:
		harness = p.view.reviewerHarness
	}
	agent := harnessName(harness)
	if p.view.switched[role] {
		if agent == "Codex" {
			agent = "OpenCode"
		} else {
			agent = "Codex"
		}
	}
	switch stage {
	case store.WorkflowStageDelegation:
		return agent + " working"
	case store.WorkflowStageIntake:
		return "Request check"
	case store.WorkflowStageRouting:
		return "Jev classifying request"
	case store.WorkflowStageAnswering:
		return agent + " answering (read-only)"
	case store.WorkflowStagePlanning:
		if p.view.routingSource == store.DecisionFallback {
			return agent + " assessing request (read-only)"
		}
		return agent + " planning"
	case store.WorkflowStageImplementation:
		return agent + " implementing"
	case store.WorkflowStageValidation:
		return "Validation"
	case store.WorkflowStageReview:
		return agent + " reviewing"
	case store.WorkflowStageRepair:
		return agent + " repairing"
	default:
		return "Workflow [redacted]"
	}
}

func activityLabel(kind activity.Kind) string {
	switch kind {
	case activity.Starting:
		return "agent starting"
	case activity.TurnStarted:
		return "agent turn started"
	case activity.CommandRunning:
		return "command running"
	case activity.CommandFinished:
		return "command finished"
	case activity.FilesChanged:
		return "file changes reported"
	case activity.ToolRunning:
		return "tool running"
	case activity.ToolFinished:
		return "tool finished"
	case activity.ToolFailed:
		return "tool failure reported"
	case activity.ResponseReceived:
		return "response received"
	case activity.StepFinished:
		return "agent step ended (not approval)"
	default:
		return "activity [redacted]"
	}
}

func (p *progressSink) writeHuman(record logRecord) {
	p.clearLine()
	if p.view.animate && !p.view.expanded && record.Code == "" {
		switch record.Type {
		case workflow.EventTypeStageStarted:
			if !p.view.sectionShown {
				view := p.terminalView()
				p.writeBytes([]byte(view.styledText("\n" + view.paragraph("── Progress ──", 2, "1;36") + view.paragraph("Details hidden · use --progress expanded or /set progress expanded", 4, "2"))))
				p.view.sectionShown = true
			}
			p.drawLive(time.Now())
			return
		case workflow.EventTypeStageCompleted:
			return
		}
	}
	label, color, message := "INFO", "36", ""
	switch record.Code {
	case "agent_activity":
		// Live activity uses the single redraw line; plain mode gets bounded lines.
		if p.view.animate {
			return
		}
		message = p.stageLabel(record.Stage) + ": " + activityLabel(record.Activity)
		if record.Activity == activity.ToolFailed {
			label, color = "WARN", "33"
		}
	case "codex_runtime_selected":
		message = "Codex runtime " + record.RuntimeVersion + " selected"
	case "no_validation_checks":
		label, color, message = "WARN", "33", "No deterministic validation checks configured; this is not a test pass."
	case "result_output_failed":
		label, color, message = "FAIL", "31", "Result could not be written; process exit 1."
	case "result_ready":
		switch record.Status {
		case store.TaskStatusResponded:
			label, color, message = "OK", "32", "Agent responded"
		case store.TaskStatusNeedsInput:
			label, color, message = "WAIT", "33", "Needs your input"
		case store.TaskStatusTimedOut:
			label, color, message = "TIMEOUT", "33", "Agent deadline expired"
		case store.TaskStatusApproved:
			label, color, message = "OK", "32", "Approved"
		case store.TaskStatusAnswered:
			label, color, message = "OK", "32", "Read-only response complete"
		case store.TaskStatusCancelled:
			label, color, message = "STOP", "33", "Cancelled or timed out"
		case store.TaskStatusRepairLimitReached:
			label, color, message = "WARN", "33", "Repair limit reached; not approved"
		default:
			label, color, message = "FAIL", "31", "Workflow failed"
		}
		if !p.view.started.IsZero() {
			message += " | elapsed " + elapsed(time.Since(p.view.started))
		}
		message += fmt.Sprintf(" | exit %d\n%s\nRun: %s", *record.ExitCode, p.view.summary, p.runID)
	case "":
		message = p.stageLabel(record.Stage)
		switch record.Type {
		case workflow.EventTypeRoutingDecided:
			if record.DecisionSource == store.DecisionFallback {
				label, color = "WARN", "33"
				reason := "unavailable"
				switch record.RoutingFallback {
				case store.RoutingInvalid:
					reason = "returned an invalid response"
				case store.RoutingLowConfidence:
					reason = "confidence was too low"
				}
				message = "Jev " + reason + "; using read-only assessment"
			} else {
				route := "[redacted]"
				switch record.Route {
				case store.RouteAnswer:
					route = "answer question (read-only)"
				case store.RoutePlan:
					route = "plan the requested change"
				case store.RouteImplement:
					route = "implement the simple change directly"
				}
				message = fmt.Sprintf("Jev → %s | confidence %.0f%%", route, record.Confidence*100)
			}
		case workflow.EventTypeStageStarted:
			label, color = "RUN", "34"
			if record.RepairAttempt > 0 {
				message += fmt.Sprintf(" | repair round %d", record.RepairAttempt)
			}
		case workflow.EventTypeStageCompleted:
			label, color = "OK", "32"
			message += " completed | " + elapsed(time.Since(p.view.stageStarted))
			if record.Stage == store.WorkflowStageValidation || record.Stage == store.WorkflowStageReview {
				label, color = "INFO", "36" // Stage completion is not passing checks or approval.
			}
		case workflow.EventTypeStageFailed:
			label, color = "FAIL", "31"
			if record.Status == store.TaskStatusCancelled {
				label, color = "STOP", "33"
			}
			if record.Status == store.TaskStatusNeedsInput {
				label, color = "WAIT", "33"
			}
			message += " stopped"
			if record.FailureCode != "" {
				message += " | " + string(record.FailureCode)
			}
		case workflow.EventTypeStageProgress:
			if record.BlockingFindings == 0 {
				return
			}
			label, color = "WARN", "33"
			message += fmt.Sprintf(" | blocking findings: %d", record.BlockingFindings)
		case workflow.EventTypeAgentRetryScheduled:
			label, color = "WAIT", "33"
			message += fmt.Sprintf(
				" | %s | retry %d in %s",
				record.ProviderKind,
				record.RetryAttempt,
				elapsed(time.Duration(record.RetryDelayMillis)*time.Millisecond+time.Second-time.Nanosecond),
			)
		case workflow.EventTypeAgentSwitched:
			label, color = "WARN", "33"
			message = "Confirmed provider switch: " + message
		case workflow.EventTypeWorkflowCompleted:
			return // Only validated final output supplies the human outcome.
		default:
			message = "Workflow event [redacted]"
		}
	default:
		message = "Workflow notice [redacted]"
	}
	view := p.terminalView()
	prefix := "[" + label + "] "
	var text strings.Builder
	for i, line := range wrapTerminal(message, view.contentWidth()-len(prefix)) {
		if i == 0 {
			text.WriteString("  " + view.paint("["+label+"]", color) + " " + line + "\n")
		} else {
			text.WriteString("  " + strings.Repeat(" ", len(prefix)) + line + "\n")
		}
	}
	p.writeBytes([]byte(view.styledText(text.String())))
}

// Summary comes from evidence counts and allowlisted statuses, never model prose.
func resultSummary(output store.TaskOutput) string {
	if output.Direct != nil {
		return "Direct delegation - native CLI response"
	}
	parts := []string{fmt.Sprintf("Agent calls: %d; repair rounds: %d", output.AgentInvocations, output.RepairAttempts)}
	if output.Validation != nil {
		passed, failed := 0, 0
		for _, check := range output.Validation.Checks {
			if check.Passed {
				passed++
			} else {
				failed++
			}
		}
		if passed+failed == 0 {
			parts = append(parts, "Latest validation: no checks configured")
		} else {
			parts = append(parts, fmt.Sprintf("Latest validation: %d passed, %d failed", passed, failed))
		}
	} else {
		parts = append(parts, "Validation: not run")
	}
	if output.LastReview != nil {
		blocking := 0
		for _, finding := range output.LastReview.Findings {
			if finding.Blocking {
				blocking++
			}
		}
		parts = append(parts, fmt.Sprintf("Latest review: %d blocking findings", blocking))
	}
	if output.Failure != nil {
		event := redactEvent(workflow.Event{Stage: output.Failure.Stage, FailureCode: output.Failure.Code})
		parts = append(parts, "Failure: "+string(event.Stage)+"/"+string(event.FailureCode))
		if provider := output.Failure.Provider; provider != nil && provider.Validate() == nil {
			parts = append(parts, "Provider: "+string(provider.Kind))
		}
	}
	return strings.Join(parts, "\n")
}

func harnessName(harness string) string {
	switch harness {
	case "opencode":
		return "OpenCode"
	case "claude":
		return "Claude"
	default:
		return "Codex"
	}
}

func (p *progressSink) terminalView() *interactiveView {
	width := 76
	if p.view.size != nil {
		if columns, tty := p.view.size(); tty && columns > 0 {
			width = max(8, columns-3)
		}
	}
	return &interactiveView{writer: p.writer, color: p.view.color, trueColor: p.view.trueColor, width: width}
}
