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
	return "\x1b[" + color + "m" + text + "\x1b[0m"
}

func (p *progressSink) stageLabel(stage store.WorkflowStage) string {
	role := stage
	if role == store.WorkflowStageRepair {
		role = store.WorkflowStageImplementation
	}
	agent := "Codex"
	if role == store.WorkflowStageImplementation || (role == store.WorkflowStagePlanning && p.view.plannerHarness == "opencode") {
		agent = "OpenCode"
	}
	if p.view.switched[role] {
		if agent == "Codex" {
			agent = "OpenCode"
		} else {
			agent = "Codex"
		}
	}
	switch stage {
	case store.WorkflowStageIntake:
		return "Workspace check"
	case store.WorkflowStagePlanning:
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
		case store.TaskStatusApproved:
			label, color, message = "OK", "32", "Approved"
		case store.TaskStatusAnswered:
			label, color, message = "OK", "32", "Answered (no implementation required)"
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
	p.writeBytes([]byte(p.paint("["+label+"]", color) + " " + message + "\n"))
}

// Summary comes from evidence counts and allowlisted statuses, never model prose.
func resultSummary(output store.TaskOutput) string {
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
