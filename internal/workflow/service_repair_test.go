package workflow_test

import (
	"reflect"
	"testing"

	"multiharness-core/internal/store"
)

func TestRunReturnsRejectedEvidenceForRepairThenApproves(t *testing.T) {
	harness := newWorkflowHarness(t)
	input := validTask(1)
	initial := implementation("initial implementation", "service.go")
	repaired := implementation("blocking finding repaired", "service_test.go")
	firstValidation := passingValidation()
	secondValidation := passingValidation()
	firstReview := rejectedReview("one blocking issue remains")
	secondReview := approvedReview("repair approved")
	harness.implementer.initial = initial
	harness.implementer.repairs = []store.ImplementationResult{repaired}
	harness.validator.reports = []store.ValidationReport{firstValidation, secondValidation}
	harness.reviewer.reviews = []store.Review{firstReview, secondReview}

	output := harness.service.Run(t.Context(), input)

	if output.Status != store.TaskStatusApproved {
		t.Fatalf("Run() status = %q, want %q; failure = %#v", output.Status, store.TaskStatusApproved, output.Failure)
	}
	if output.RepairAttempts != 1 {
		t.Fatalf("Run() repair attempts = %d, want 1", output.RepairAttempts)
	}
	if err := output.Validate(); err != nil {
		t.Fatalf("Run() output validation error = %v", err)
	}

	wantCalls := []string{
		"workspace", "plan", "implement", "validate", "review", "repair", "validate", "review",
	}
	if got := harness.calls.snapshot(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("call order = %v, want %v", got, wantCalls)
	}

	if len(harness.implementer.repairCalls) != 1 {
		t.Fatalf("ApplyReview() calls = %d, want 1", len(harness.implementer.repairCalls))
	}
	repairRequest := harness.implementer.repairCalls[0]
	if !reflect.DeepEqual(repairRequest.Input, input) ||
		!reflect.DeepEqual(repairRequest.Plan, validPlan()) ||
		!reflect.DeepEqual(repairRequest.Implementation, initial) ||
		!reflect.DeepEqual(repairRequest.Validation, firstValidation) ||
		!reflect.DeepEqual(repairRequest.Review, firstReview) {
		t.Fatalf("ApplyReview() request did not preserve the rejected evidence: %#v", repairRequest)
	}

	if len(harness.reviewer.requests) != 2 {
		t.Fatalf("Review() calls = %d, want 2", len(harness.reviewer.requests))
	}
	secondReviewRequest := harness.reviewer.requests[1]
	if !reflect.DeepEqual(secondReviewRequest.Implementation, repaired) {
		t.Fatalf("second Review() implementation = %#v, want repaired implementation", secondReviewRequest.Implementation)
	}
	if !reflect.DeepEqual(secondReviewRequest.Validation, secondValidation) {
		t.Fatalf("second Review() validation = %#v, want second validation", secondReviewRequest.Validation)
	}
	if output.Implementation == nil || !reflect.DeepEqual(*output.Implementation, repaired) {
		t.Fatalf("Run() implementation evidence = %#v, want latest repair", output.Implementation)
	}
	secondReview.Findings = []store.ReviewFinding{}
	secondReview.Suggestions = []string{}
	if output.LastReview == nil || !reflect.DeepEqual(*output.LastReview, secondReview) {
		t.Fatalf("Run() review evidence = %#v, want final review", output.LastReview)
	}
}

func TestRunStopsAfterConfiguredRepairAttemptsWithoutClaimingSuccess(t *testing.T) {
	harness := newWorkflowHarness(t)
	input := validTask(2)
	firstRepair := implementation("first repair", "first.go")
	secondRepair := implementation("second repair", "second.go")
	firstReview := rejectedReview("first rejection")
	secondReview := rejectedReview("second rejection")
	finalReview := rejectedReview("final rejection")
	harness.implementer.repairs = []store.ImplementationResult{firstRepair, secondRepair}
	harness.validator.reports = []store.ValidationReport{
		passingValidation(), passingValidation(), passingValidation(),
	}
	harness.reviewer.reviews = []store.Review{firstReview, secondReview, finalReview}

	output := harness.service.Run(t.Context(), input)

	if output.Status != store.TaskStatusRepairLimitReached {
		t.Fatalf(
			"Run() status = %q, want %q; failure = %#v",
			output.Status,
			store.TaskStatusRepairLimitReached,
			output.Failure,
		)
	}
	if output.RepairAttempts != 2 {
		t.Fatalf("Run() repair attempts = %d, want 2", output.RepairAttempts)
	}
	if output.Summary != finalReview.Summary {
		t.Fatalf("Run() summary = %q, want final rejected review summary %q", output.Summary, finalReview.Summary)
	}
	if err := output.Validate(); err != nil {
		t.Fatalf("Run() output validation error = %v", err)
	}

	wantCalls := []string{
		"workspace", "plan", "implement",
		"validate", "review", "repair",
		"validate", "review", "repair",
		"validate", "review",
	}
	if got := harness.calls.snapshot(); !reflect.DeepEqual(got, wantCalls) {
		t.Fatalf("call order = %v, want %v", got, wantCalls)
	}
	if len(harness.implementer.repairCalls) != input.MaxRepairAttempts {
		t.Fatalf("ApplyReview() calls = %d, want %d", len(harness.implementer.repairCalls), input.MaxRepairAttempts)
	}
	if output.Implementation == nil || !reflect.DeepEqual(*output.Implementation, secondRepair) {
		t.Fatalf("Run() implementation evidence = %#v, want latest repair", output.Implementation)
	}
	finalReview.Suggestions = []string{}
	if output.LastReview == nil || !reflect.DeepEqual(*output.LastReview, finalReview) {
		t.Fatalf("Run() review evidence = %#v, want final rejected review", output.LastReview)
	}
}
