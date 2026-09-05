package store

import "testing"

func TestReviewValidate(t *testing.T) {
	tests := []struct {
		name      string
		review    Review
		wantField string
	}{
		{name: "approved", review: validApprovedReview()},
		{name: "rejected with blocking finding", review: validRejectedReview()},
		{
			name: "approved with blocking finding",
			review: Review{
				Approved: true,
				Summary:  "Inconsistent decision",
				Findings: validRejectedReview().Findings,
			},
			wantField: "approved",
		},
		{
			name: "rejected without blocking finding",
			review: Review{
				Summary: "No blocking evidence",
				Findings: []ReviewFinding{
					{Severity: FindingSeverityWarning, Description: "Optional cleanup"},
				},
			},
			wantField: "findings",
		},
		{
			name: "unsupported severity",
			review: Review{
				Approved: true,
				Summary:  "Invalid finding",
				Findings: []ReviewFinding{
					{Severity: "urgent", Description: "Invalid severity"},
				},
			},
			wantField: "findings[0].severity",
		},
		{
			name: "blocking finding without action",
			review: Review{
				Summary: "Missing action",
				Findings: []ReviewFinding{
					{Severity: FindingSeverityError, Blocking: true, Description: "Broken behavior"},
				},
			},
			wantField: "findings[0].required_action",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertValidationResult(t, test.review.Validate(), test.wantField)
		})
	}
}

func TestReviewRequestValidate(t *testing.T) {
	request := validReviewRequest()
	invalidRequest := request
	invalidRequest.Validation.Passed = false

	assertValidationResult(t, request.Validate(), "")
	assertValidationResult(t, invalidRequest.Validate(), "validation.passed")
}

func TestRepairRequestValidate(t *testing.T) {
	reviewRequest := validReviewRequest()
	request := RepairRequest{
		Input:          reviewRequest.Input,
		Plan:           reviewRequest.Plan,
		Implementation: reviewRequest.Implementation,
		Validation:     reviewRequest.Validation,
		Review:         validRejectedReview(),
	}
	approvedRequest := request
	approvedRequest.Review = validApprovedReview()

	assertValidationResult(t, request.Validate(), "")
	assertValidationResult(t, approvedRequest.Validate(), "review.approved")
}

func validReviewRequest() ReviewRequest {
	return ReviewRequest{
		Input:          validTaskInput(),
		Plan:           validPlan(),
		Implementation: validImplementationResult(),
		Validation:     validValidationReport(),
	}
}
