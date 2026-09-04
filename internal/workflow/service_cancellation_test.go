package workflow_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"multiharness-core/internal/store"
)

func TestRunDoesNotStartWorkForPreCancelledContext(t *testing.T) {
	harness := newWorkflowHarness(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	output := harness.service.Run(ctx, validTask(0))

	assertCancelledAtStage(t, output, store.WorkflowStageIntake)
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
	harness.workspace.validate = func(actual context.Context, _ string) error {
		checkContext(actual)
		return nil
	}
	harness.planner.run = func(actual context.Context, _ store.TaskInput) (store.Plan, error) {
		checkContext(actual)
		return validPlan(), nil
	}
	harness.implementer.implement = func(actual context.Context, _ store.ImplementationRequest) (store.ImplementationResult, error) {
		checkContext(actual)
		return implementation("implemented", "service.go"), nil
	}
	harness.implementer.repair = func(actual context.Context, _ store.RepairRequest) (store.ImplementationResult, error) {
		checkContext(actual)
		return implementation("repaired", "service.go"), nil
	}
	harness.validator.validate = func(actual context.Context, _ store.ValidationRequest) (store.ValidationReport, error) {
		checkContext(actual)
		return passingValidation(), nil
	}
	reviews := 0
	harness.reviewer.review = func(actual context.Context, _ store.ReviewRequest) (store.Review, error) {
		checkContext(actual)
		reviews++
		if reviews == 1 {
			return rejectedReview("repair required"), nil
		}
		return approvedReview("repair approved"), nil
	}
	output := harness.service.Run(ctx, validTask(1))
	if output.Status != store.TaskStatusApproved || output.RepairAttempts != 1 || reviews != 2 {
		t.Fatalf("repair loop did not complete: %#v", output)
	}
}

func TestRunPropagatesCancellationToTheActivePort(t *testing.T) {
	harness := newWorkflowHarness(t)
	started := make(chan struct{})
	harness.planner.run = func(ctx context.Context, _ store.TaskInput) (store.Plan, error) {
		close(started)
		<-ctx.Done()
		return store.Plan{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan store.TaskOutput, 1)
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
		assertCancelledAtStage(t, output, store.WorkflowStagePlanning)
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not return after context cancellation")
	}
	if got, want := harness.calls.snapshot(), []string{"workspace", "plan"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("call order = %v, want %v", got, want)
	}
}

func TestRunHonorsCancellationEvenWhenAPortReturnsSuccess(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*workflowHarness, context.CancelFunc)
		wantStage store.WorkflowStage
		wantCalls []string
	}{
		{
			name: "workspace",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.workspace.validate = func(context.Context, string) error {
					cancel()
					return nil
				}
			},
			wantStage: store.WorkflowStageIntake,
			wantCalls: []string{"workspace"},
		},
		{
			name: "planner",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.planner.run = func(context.Context, store.TaskInput) (store.Plan, error) {
					cancel()
					return validPlan(), nil
				}
			},
			wantStage: store.WorkflowStagePlanning,
			wantCalls: []string{"workspace", "plan"},
		},
		{
			name: "implementer",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.implementer.implement = func(
					context.Context,
					store.ImplementationRequest,
				) (store.ImplementationResult, error) {
					cancel()
					return implementation("implemented", "service.go"), nil
				}
			},
			wantStage: store.WorkflowStageImplementation,
			wantCalls: []string{"workspace", "plan", "implement"},
		},
		{
			name: "validator",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.validator.validate = func(
					context.Context,
					store.ValidationRequest,
				) (store.ValidationReport, error) {
					cancel()
					return passingValidation(), nil
				}
			},
			wantStage: store.WorkflowStageValidation,
			wantCalls: []string{"workspace", "plan", "implement", "validate"},
		},
		{
			name: "reviewer",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.reviewer.review = func(
					context.Context,
					store.ReviewRequest,
				) (store.Review, error) {
					cancel()
					return approvedReview("approved"), nil
				}
			},
			wantStage: store.WorkflowStageReview,
			wantCalls: []string{"workspace", "plan", "implement", "validate", "review"},
		},
		{
			name: "repair",
			setup: func(harness *workflowHarness, cancel context.CancelFunc) {
				harness.reviewer.reviews = []store.Review{rejectedReview("repair required")}
				harness.implementer.repair = func(
					context.Context,
					store.RepairRequest,
				) (store.ImplementationResult, error) {
					cancel()
					return implementation("repaired", "service.go"), nil
				}
			},
			wantStage: store.WorkflowStageRepair,
			wantCalls: []string{"workspace", "plan", "implement", "validate", "review", "repair"},
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

func assertCancelledAtStage(t *testing.T, output store.TaskOutput, stage store.WorkflowStage) {
	t.Helper()
	if output.Status != store.TaskStatusCancelled {
		t.Fatalf("Run() status = %q, want %q; failure = %#v", output.Status, store.TaskStatusCancelled, output.Failure)
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
