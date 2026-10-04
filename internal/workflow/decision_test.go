package workflow_test

import (
	"context"
	"errors"
	"testing"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

type reviewDecisionStub struct{ err error }

func (d reviewDecisionStub) DecidePlanning(context.Context, contract.TaskInput) (contract.PlanningDecision, error) {
	return contract.PlanningDecision{Route: contract.RoutePlan, Source: contract.DecisionJev, NeedsPlanning: true}, nil
}
func (d reviewDecisionStub) DecideReview(context.Context, contract.ReviewRequest) (contract.ReviewDecision, error) {
	return contract.ReviewDecision{Approved: true}, d.err
}

func TestDecisionReviewPreservesValidationAndFallbackBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name         string
		validation   contract.ValidationReport
		decisionErr  error
		wantReviewer bool
	}{
		{name: "no checks", validation: contract.ValidationReport{Passed: true}, wantReviewer: true},
		{name: "decision failed", validation: passingValidation(), decisionErr: errors.New("router unavailable"), wantReviewer: true},
		{name: "validated approval", validation: passingValidation()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newWorkflowHarness(t)
			h.validator.reports = []contract.ValidationReport{tc.validation}
			service, err := workflow.NewService(workflow.Dependencies{
				Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer,
				Validator: h.validator, Reviewer: h.reviewer, Events: h.events,
				DecisionMaker: reviewDecisionStub{err: tc.decisionErr},
			})
			if err != nil {
				t.Fatal(err)
			}
			output := service.Run(t.Context(), validTask(0))
			if output.Status != contract.TaskStatusApproved {
				t.Fatalf("status=%s failure=%+v", output.Status, output.Failure)
			}
			if err := output.Validate(); err != nil {
				t.Fatal(err)
			}
			if got := len(h.reviewer.requests) > 0; got != tc.wantReviewer {
				t.Fatalf("reviewer called=%v, want %v", got, tc.wantReviewer)
			}
			if tc.wantReviewer && output.Summary != "approved by review" {
				t.Fatalf("lost independent verdict: %s", output.Summary)
			}
		})
	}
}
