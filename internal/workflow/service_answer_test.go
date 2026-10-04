package workflow_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

func answerPlan() contract.Plan {
	return contract.Plan{Action: contract.PlanActionAnswer, Summary: "explain the code", Answer: "The workflow uses explicit Go stages."}
}

func TestRunAnswersWithoutCallingCodingPorts(t *testing.T) {
	harness := newWorkflowHarness(t)
	harness.planner.plan = answerPlan()
	harness.workspace.acquireErr = errors.New("baseline must not be captured for an answer")
	output := harness.service.Run(t.Context(), validTask(3))
	if output.Status != contract.TaskStatusAnswered || output.Summary != answerPlan().Answer {
		t.Fatalf("output: %#v", output)
	}
	if err := output.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := harness.calls.snapshot(); !reflect.DeepEqual(got, []string{"plan"}) {
		t.Fatalf("unexpected coding calls: %v", got)
	}
	if harness.workspace.session != nil {
		t.Fatal("answer acquired a workspace lease")
	}
	events := harness.events.snapshot()
	if len(events) != 5 || events[4].Type != workflow.EventTypeWorkflowCompleted || events[4].Stage != contract.WorkflowStagePlanning || events[4].Status != contract.TaskStatusAnswered {
		t.Fatalf("events: %#v", events)
	}
}

func TestAnswerValidatesOutputAndHonorsCancellation(t *testing.T) {
	for _, scenario := range []string{"cancelled", "invalid answer"} {
		t.Run(
			scenario,
			func(t *testing.T) {
				harness := newWorkflowHarness(t)
				harness.workspace.session = newFakeWorkspaceSession()
				harness.planner.plan = answerPlan()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				switch scenario {
				case "cancelled":
					harness.planner.run = func(context.Context, contract.TaskInput) (contract.Plan, error) { cancel(); return answerPlan(), nil }
				case "invalid answer":
					harness.planner.plan.Answer = " "
				}
				output := harness.service.Run(ctx, validTask(1))
				if output.Status != contract.TaskStatusFailed && output.Status != contract.TaskStatusCancelled {
					t.Fatalf("unsafe answer: %#v", output)
				}
				if err := output.Validate(); err != nil {
					t.Fatal(err)
				}
				if len(harness.implementer.implementationCalls) != 0 {
					t.Fatal("called implementation")
				}
			},
		)
	}
}
