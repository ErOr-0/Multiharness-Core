package workflow_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func answerPlan() store.Plan {
	return store.Plan{Action: store.PlanActionAnswer, Summary: "explain the code", Answer: "The workflow uses explicit Go stages."}
}

func TestRunAnswersWithoutCallingCodingPorts(t *testing.T) {
	harness := newWorkflowHarness(t)
	harness.planner.plan = answerPlan()
	output := harness.service.Run(t.Context(), validTask(3))
	if output.Status != store.TaskStatusAnswered || output.Summary != answerPlan().Answer {
		t.Fatalf("output: %#v", output)
	}
	if err := output.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := harness.calls.snapshot(); !reflect.DeepEqual(got, []string{"workspace", "plan"}) {
		t.Fatalf("unexpected coding calls: %v", got)
	}
	if !harness.workspace.session.closed {
		t.Fatal("answer leaked workspace lease")
	}
	events := harness.events.snapshot()
	if len(events) != 5 || events[4].Type != workflow.EventTypeWorkflowCompleted || events[4].Stage != store.WorkflowStagePlanning || events[4].Status != store.TaskStatusAnswered {
		t.Fatalf("events: %#v", events)
	}
	_ = marshalOutput(t, output)
}

func TestAnswerCannotBypassWorkspaceSafetyOrCancellation(t *testing.T) {
	for _, scenario := range []string{"changed workspace", "incomplete inspection", "close failure", "cancelled", "invalid answer"} {
		t.Run(scenario, func(t *testing.T) {
			harness := newWorkflowHarness(t)
			harness.workspace.session = newFakeWorkspaceSession()
			harness.planner.plan = answerPlan()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch scenario {
			case "changed workspace":
				harness.workspace.session.current.Current.Fingerprint = "modified"
			case "incomplete inspection":
				harness.workspace.session.current.Complete = false
			case "close failure":
				harness.workspace.session.closeErr = errors.New("cannot release lease")
			case "cancelled":
				harness.planner.run = func(context.Context, store.TaskInput) (store.Plan, error) { cancel(); return answerPlan(), nil }
			case "invalid answer":
				harness.planner.plan.Answer = " "
			}
			output := harness.service.Run(ctx, validTask(1))
			if output.Status != store.TaskStatusFailed && output.Status != store.TaskStatusCancelled {
				t.Fatalf("unsafe answer: %#v", output)
			}
			if err := output.Validate(); err != nil {
				t.Fatal(err)
			}
			if len(harness.implementer.implementationCalls) != 0 {
				t.Fatal("called implementation")
			}
		})
	}
}
