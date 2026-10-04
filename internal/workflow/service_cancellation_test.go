package workflow_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

type eventHook func(workflow.Event)

func (hook eventHook) Publish(event workflow.Event) { hook(event) }

func TestRunHonorsCancellationBeforePublishingTerminalOutcome(t *testing.T) {
	for _, outcome := range []string{"answer", "approval", "repair limit"} {
		for _, trigger := range []string{"stage completion", "workspace release"} {
			if outcome == "answer" && trigger == "workspace release" {
				continue
			}
			t.Run(outcome+"/"+trigger, func(t *testing.T) {
				h := newWorkflowHarness(t)
				h.workspace.session = newFakeWorkspaceSession()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				stage := contract.WorkflowStageReview
				switch outcome {
				case "answer":
					h.planner.plan = contract.Plan{Action: contract.PlanActionAnswer, Summary: "answer", Answer: "done"}
					stage = contract.WorkflowStagePlanning
				case "repair limit":
					h.reviewer.reviews = []contract.Review{rejectedReview("repair required")}
				}
				if trigger == "workspace release" {
					h.workspace.session.closeHook = cancel
				}
				service, err := workflow.NewService(workflow.Dependencies{
					Workspace:   h.workspace,
					Planner:     h.planner,
					Implementer: h.implementer,
					Validator:   h.validator,
					Reviewer:    h.reviewer,
					Events: eventHook(func(event workflow.Event) {
						h.events.Publish(event)
						if trigger == "stage completion" && event.Type == workflow.EventTypeStageCompleted && event.Stage == stage {
							cancel()
						}
					}),
				})
				if err != nil {
					t.Fatal(err)
				}
				output := service.Run(ctx, validTask(0))
				assertCancelledAtStage(t, output, stage)
				if outcome != "answer" && h.workspace.session != nil && !h.workspace.session.closed {
					t.Fatal("cancelled run leaked workspace lease")
				}
				completed := 0
				for _, event := range h.events.snapshot() {
					if event.Type == workflow.EventTypeWorkflowCompleted {
						completed++
						if event.Status != contract.TaskStatusCancelled {
							t.Fatalf("published terminal status %q after cancellation", event.Status)
						}
					}
				}
				if completed != 1 {
					t.Fatalf("published %d terminal events, want one", completed)
				}
			})
		}
	}
}

func TestRunStopsBetweenStagesWhenCompletionEventCancelsContext(t *testing.T) {
	for _, test := range []struct {
		after, next contract.WorkflowStage
		wantCalls   []string
	}{
		{contract.WorkflowStageIntake, contract.WorkflowStagePlanning, nil},
		{contract.WorkflowStagePlanning, contract.WorkflowStageImplementation, []string{"plan"}},
		{contract.WorkflowStageImplementation, contract.WorkflowStageValidation, []string{"plan", "workspace", "implement"}},
		{contract.WorkflowStageValidation, contract.WorkflowStageReview, []string{"plan", "workspace", "implement", "validate"}},
		{contract.WorkflowStageReview, contract.WorkflowStageRepair, []string{"plan", "workspace", "implement", "validate", "review"}},
		{
			contract.WorkflowStageRepair,
			contract.WorkflowStageValidation,
			[]string{"plan", "workspace", "implement", "validate", "review", "repair"},
		},
	} {
		t.Run(
			string(test.after),
			func(t *testing.T) {
				h := newWorkflowHarness(t)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				h.reviewer.reviews = []contract.Review{rejectedReview("repair required")}
				h.implementer.repairs = []contract.ImplementationResult{implementation("repaired", "service.go")}
				service, err := workflow.NewService(workflow.Dependencies{
					Workspace:   h.workspace,
					Planner:     h.planner,
					Implementer: h.implementer,
					Validator:   h.validator,
					Reviewer:    h.reviewer,
					Events: eventHook(func(event workflow.Event) {
						h.events.Publish(event)
						if event.Type == workflow.EventTypeStageCompleted && event.Stage == test.after {
							cancel()
						}
					}),
				})
				if err != nil {
					t.Fatal(err)
				}
				output := service.Run(ctx, validTask(1))
				assertCancelledAtStage(t, output, test.next)
				if got := h.calls.snapshot(); !reflect.DeepEqual(got, test.wantCalls) {
					t.Fatalf("port calls after cancellation: %v; want %v", got, test.wantCalls)
				}
				if h.workspace.session != nil && !h.workspace.session.closed {
					t.Fatal("cancelled handoff leaked workspace lease")
				}
				events := h.events.snapshot()
				if last := events[len(events)-1]; last.Type != workflow.EventTypeWorkflowCompleted || last.Status != contract.TaskStatusCancelled {
					t.Fatalf("incorrect terminal event: %+v", last)
				}
			},
		)
	}
}

func TestRunDoesNotStartWorkForPreCancelledContext(t *testing.T) {
	harness := newWorkflowHarness(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	output := harness.service.Run(ctx, validTask(0))

	assertCancelledAtStage(t, output, contract.WorkflowStageIntake)
	if got := harness.calls.snapshot(); len(got) != 0 {
		t.Fatalf("port calls = %v, want none", got)
	}
}

func TestRunPreservesCallerContextAcrossTheRepairLoop(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(t.Context(), contextKey{}, "run-context"), time.Minute)
	defer cancel()
	deadline, _ := ctx.Deadline()
	checkContext := func(actual context.Context) {
		t.Helper()
		if actual.Value(contextKey{}) != "run-context" {
			t.Fatal("caller context value was lost")
		}
		if actualDeadline, ok := actual.Deadline(); !ok || !actualDeadline.Equal(deadline) {
			t.Fatal("caller deadline was lost")
		}
		if actual.Done() != ctx.Done() {
			t.Fatal("caller cancellation was replaced")
		}
	}
	harness := newWorkflowHarness(t)
	harness.workspace.acquire = func(actual context.Context, _ string) error {
		checkContext(actual)
		return nil
	}
	harness.planner.run = func(actual context.Context, _ contract.TaskInput) (contract.Plan, error) {
		checkContext(actual)
		return validPlan(), nil
	}
	harness.implementer.implement = func(actual context.Context, _ contract.ImplementationRequest) (contract.ImplementationResult, error) {
		checkContext(actual)
		return implementation("implemented", "service.go"), nil
	}
	harness.implementer.repair = func(actual context.Context, _ contract.RepairRequest) (contract.ImplementationResult, error) {
		checkContext(actual)
		return implementation("repaired", "service.go"), nil
	}
	harness.validator.validate = func(actual context.Context, _ contract.ValidationRequest) (contract.ValidationReport, error) {
		checkContext(actual)
		return passingValidation(), nil
	}
	reviews := 0
	harness.reviewer.review = func(actual context.Context, _ contract.ReviewRequest) (contract.Review, error) {
		checkContext(actual)
		reviews++
		if reviews == 1 {
			return rejectedReview("repair required"), nil
		}
		return approvedReview("repair approved"), nil
	}
	output := harness.service.Run(ctx, validTask(1))
	if output.Status != contract.TaskStatusApproved || output.RepairAttempts != 1 || reviews != 2 {
		t.Fatalf("repair loop did not complete: %#v", output)
	}
}

func TestRunPropagatesCancellationToTheActivePort(t *testing.T) {
	harness := newWorkflowHarness(t)
	started := make(chan struct{})
	harness.planner.run = func(ctx context.Context, _ contract.TaskInput) (contract.Plan, error) {
		close(started)
		<-ctx.Done()
		return contract.Plan{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan contract.TaskOutput, 1)
	go func() {
		result <- harness.service.Run(ctx, validTask(0))
	}()

	select {
	case <-started:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("planner did not receive the workflow context")
	}

	select {
	case output := <-result:
		assertCancelledAtStage(t, output, contract.WorkflowStagePlanning)
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
	if got, want := harness.calls.snapshot(), []string{"plan"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
}

func TestRunHonorsCancellationEvenWhenAPortReturnsSuccess(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*workflowHarness, context.CancelFunc)
		wantStage contract.WorkflowStage
		wantCalls []string
	}{
		{
			name: "workspace",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.workspace.acquire = func(context.Context, string) error {
					cancel()
					return nil
				}
			},
			wantStage: contract.WorkflowStageImplementation,
			wantCalls: []string{"plan", "workspace"},
		},
		{
			name: "planner",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.planner.run = func(context.Context, contract.TaskInput) (contract.Plan, error) {
					cancel()
					return validPlan(), nil
				}
			},
			wantStage: contract.WorkflowStagePlanning,
			wantCalls: []string{"plan"},
		},
		{
			name: "implementer",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.implementer.implement = func(
					context.Context,
					contract.ImplementationRequest,
				) (contract.ImplementationResult, error) {
					cancel()
					return implementation("implemented", "service.go"), nil
				}
			},
			wantStage: contract.WorkflowStageImplementation,
			wantCalls: []string{"plan", "workspace", "implement"},
		},
		{
			name: "validator",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.validator.validate = func(
					context.Context,
					contract.ValidationRequest,
				) (contract.ValidationReport, error) {
					cancel()
					return passingValidation(), nil
				}
			},
			wantStage: contract.WorkflowStageValidation,
			wantCalls: []string{"plan", "workspace", "implement", "validate"},
		},
		{
			name: "reviewer",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.reviewer.review = func(
					context.Context,
					contract.ReviewRequest,
				) (contract.Review, error) {
					cancel()
					return approvedReview("approved"), nil
				}
			},
			wantStage: contract.WorkflowStageReview,
			wantCalls: []string{"plan", "workspace", "implement", "validate", "review"},
		},
		{
			name: "repair",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.reviewer.reviews = []contract.Review{rejectedReview("repair required")}
				harness.implementer.repair = func(
					context.Context,
					contract.RepairRequest,
				) (contract.ImplementationResult, error) {
					cancel()
					return implementation("repaired", "service.go"), nil
				}
			},
			wantStage: contract.WorkflowStageRepair,
			wantCalls: []string{"plan", "workspace", "implement", "validate", "review", "repair"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newWorkflowHarness(t)
			ctx, cancel := context.WithCancel(t.Context())
			test.setup(harness, cancel)

			output := harness.service.Run(ctx, validTask(1))

			assertCancelledAtStage(t, output, test.wantStage)
			if got := harness.calls.snapshot(); !reflect.DeepEqual(got, test.wantCalls) {
				t.Fatalf("call order = %v, want %v", got, test.wantCalls)
			}
		})
	}
}

func assertCancelledAtStage(t *testing.T, output contract.TaskOutput, stage contract.WorkflowStage) {
	t.Helper()
	if output.Status != contract.TaskStatusCancelled {
		t.Fatalf("Run() status = %q, want %q; failure = %#v", output.Status, contract.TaskStatusCancelled, output.Failure)
	}
	if output.Failure != nil {
		t.Fatalf("Run() failure = %#v, want nil for cancellation", output.Failure)
	}
	if want := "workflow cancelled during " + string(stage); len(output.Summary) < len(want) || output.Summary[:len(want)] != want {
		t.Fatalf("Run() summary = %q, want prefix %q", output.Summary, want)
	}
	if err := output.Validate(); err != nil {
		t.Fatalf("Run() output validation error = %v", err)
	}
}
