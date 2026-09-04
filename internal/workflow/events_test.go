package workflow_test

import (
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func TestRunEmitsOrderedStructuredEventsForARepairCycle(t *testing.T) {
	harness := newWorkflowHarness(t)
	harness.reviewer.reviews = []store.Review{
		rejectedReview("repair required"),
		approvedReview("repair approved"),
	}
	harness.implementer.repairs = []store.ImplementationResult{
		implementation("repaired", "service.go"),
	}
	harness.validator.reports = []store.ValidationReport{passingValidation(), passingValidation()}

	output := harness.service.Run(t.Context(), validTask(1))
	if output.Status != store.TaskStatusApproved {
		t.Fatalf("Run() status = %q, want %q", output.Status, store.TaskStatusApproved)
	}

	want := []workflow.Event{
		{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageIntake},
		{Type: workflow.EventTypeStageCompleted, Stage: store.WorkflowStageIntake},
		{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStagePlanning},
		{Type: workflow.EventTypeStageCompleted, Stage: store.WorkflowStagePlanning},
		{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageImplementation},
		{Type: workflow.EventTypeStageCompleted, Stage: store.WorkflowStageImplementation},
		{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageValidation},
		{Type: workflow.EventTypeStageCompleted, Stage: store.WorkflowStageValidation},
		{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageReview},
		{
			Type:             workflow.EventTypeStageProgress,
			Stage:            store.WorkflowStageReview,
			BlockingFindings: 1,
		},
		{Type: workflow.EventTypeStageCompleted, Stage: store.WorkflowStageReview},
		{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageRepair, RepairAttempt: 1},
		{Type: workflow.EventTypeStageCompleted, Stage: store.WorkflowStageRepair, RepairAttempt: 1},
		{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageValidation, RepairAttempt: 1},
		{Type: workflow.EventTypeStageCompleted, Stage: store.WorkflowStageValidation, RepairAttempt: 1},
		{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageReview, RepairAttempt: 1},
		{Type: workflow.EventTypeStageCompleted, Stage: store.WorkflowStageReview, RepairAttempt: 1},
		{
			Type:   workflow.EventTypeWorkflowCompleted,
			Stage:  store.WorkflowStageReview,
			Status: store.TaskStatusApproved,
		},
	}

	events := harness.events.snapshot()
	if len(events) != len(want) {
		t.Fatalf("event count = %d, want %d; events = %#v", len(events), len(want), events)
	}
	for index := range want {
		if events[index].Sequence != index+1 {
			t.Fatalf("event[%d].Sequence = %d, want %d", index, events[index].Sequence, index+1)
		}
		want[index].Sequence = index + 1
		if events[index] != want[index] {
			t.Fatalf("event[%d] = %#v, want %#v", index, events[index], want[index])
		}
	}
}

func TestRunEmitsStageFailureAndTerminalEvent(t *testing.T) {
	harness := newWorkflowHarness(t)
	harness.planner.plan = store.Plan{}

	output := harness.service.Run(t.Context(), validTask(0))
	if output.Status != store.TaskStatusFailed {
		t.Fatalf("Run() status = %q, want %q", output.Status, store.TaskStatusFailed)
	}

	events := harness.events.snapshot()
	if len(events) < 2 {
		t.Fatalf("event count = %d, want at least 2", len(events))
	}
	failure := events[len(events)-2]
	if failure.Type != workflow.EventTypeStageFailed ||
		failure.Stage != store.WorkflowStagePlanning ||
		failure.Status != store.TaskStatusFailed ||
		failure.FailureCode != store.FailureCodeInvalidOutput {
		t.Fatalf("failure event = %#v", failure)
	}
	terminal := events[len(events)-1]
	if terminal.Type != workflow.EventTypeWorkflowCompleted ||
		terminal.Stage != store.WorkflowStagePlanning ||
		terminal.Status != store.TaskStatusFailed {
		t.Fatalf("terminal event = %#v", terminal)
	}
}
