package workflow_test

import (
	"reflect"
	"testing"

	"multiharness-core/internal/store"
)

func TestRunApprovesFromPlanImplementationValidationAndReviewEvidence(t *testing.T) {
	harness := newWorkflowHarness(t)
	input := validTask(0)

	output := harness.service.Run(t.Context(), input)

	if output.Status != store.TaskStatusApproved {
		t.Fatalf("Run() status = %q, want %q; failure = %#v", output.Status, store.TaskStatusApproved, output.Failure)
	}
	if output.Summary != "approved by review" {
		t.Fatalf("Run() summary = %q, want reviewer summary", output.Summary)
	}
	if output.RepairAttempts != 0 {
		t.Fatalf("Run() repair attempts = %d, want 0", output.RepairAttempts)
	}
	if err := output.Validate(); err != nil {
		t.Fatalf("Run() output validation error = %v", err)
	}

	wantCalls := []string{"workspace", "plan", "implement", "validate", "review"}
	if got := harness.calls.snapshot(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("call order = %v, want %v", got, wantCalls)
	}

	if len(harness.implementer.implementationCalls) != 1 {
		t.Fatalf("Implement() calls = %d, want 1", len(harness.implementer.implementationCalls))
	}
	implementationRequest := harness.implementer.implementationCalls[0]
	if !reflect.DeepEqual(implementationRequest.Input, input) {
		t.Fatalf("Implement() input = %#v, want %#v", implementationRequest.Input, input)
	}
	if !reflect.DeepEqual(implementationRequest.Plan, validPlan()) {
		t.Fatalf("Implement() plan = %#v, want %#v", implementationRequest.Plan, validPlan())
	}

	if output.Plan == nil || !reflect.DeepEqual(*output.Plan, validPlan()) {
		t.Fatalf("Run() plan evidence = %#v, want %#v", output.Plan, validPlan())
	}
	if output.Implementation == nil || output.Implementation.Summary != "initial implementation" {
		t.Fatalf("Run() implementation evidence = %#v", output.Implementation)
	}
	if output.Validation == nil || !output.Validation.Passed {
		t.Fatalf("Run() validation evidence = %#v", output.Validation)
	}
	if output.LastReview == nil || !output.LastReview.Approved {
		t.Fatalf("Run() review evidence = %#v", output.LastReview)
	}
}
