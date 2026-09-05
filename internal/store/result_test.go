package store

import "testing"

func TestTaskFailureValidate(t *testing.T) {
	tests := []struct {
		name      string
		failure   TaskFailure
		wantField string
	}{
		{
			name: "valid",
			failure: TaskFailure{
				Stage:   WorkflowStagePlanning,
				Code:    FailureCodeAgent,
				Message: "planner failed",
			},
		},
		{
			name:      "invalid stage",
			failure:   TaskFailure{Stage: "unknown", Code: FailureCodeAgent, Message: "failed"},
			wantField: "stage",
		},
		{
			name:      "invalid code",
			failure:   TaskFailure{Stage: WorkflowStagePlanning, Code: "unknown", Message: "failed"},
			wantField: "code",
		},
		{
			name:      "blank message",
			failure:   TaskFailure{Stage: WorkflowStagePlanning, Code: FailureCodeAgent},
			wantField: "message",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertValidationResult(t, test.failure.Validate(), test.wantField)
		})
	}
}

func TestTaskOutputValidateTerminalStatuses(t *testing.T) {
	plan := validPlan()
	implementation := validImplementationResult()
	validation := validValidationReport()
	failedValidation := ValidationReport{
		Checks: []ValidationEvidence{{Command: "go test ./...", ExitCode: 1, Output: "failed"}},
	}
	approved := validApprovedReview()
	rejected := validRejectedReview()

	tests := []struct {
		name      string
		output    TaskOutput
		wantField string
	}{
		{
			name: "approved",
			output: TaskOutput{
				Status: TaskStatusApproved, Summary: "Approved", Plan: &plan,
				Implementation: &implementation, Validation: &validation, LastReview: &approved,
			},
		},
		{
			name: "failed",
			output: TaskOutput{
				Status: TaskStatusFailed, Summary: "Planner failed",
				Failure: &TaskFailure{Stage: WorkflowStagePlanning, Code: FailureCodeAgent, Message: "planner failed"},
			},
		},
		{name: "cancelled before planning", output: TaskOutput{Status: TaskStatusCancelled, Summary: "Cancelled"}},
		{
			name: "repair limit reached",
			output: TaskOutput{
				Status: TaskStatusRepairLimitReached, Summary: "Blocking issue remains", Plan: &plan,
				Implementation: &implementation, Validation: &failedValidation, LastReview: &rejected,
				RepairAttempts: 2,
			},
		},
		{
			name:      "approved without evidence",
			output:    TaskOutput{Status: TaskStatusApproved, Summary: "Missing"},
			wantField: "plan",
		},
		{
			name: "approved with failed validation",
			output: TaskOutput{
				Status: TaskStatusApproved, Summary: "Inconsistent", Plan: &plan,
				Implementation: &implementation, Validation: &failedValidation, LastReview: &approved,
			},
			wantField: "validation.passed",
		},
		{name: "failed without failure", output: TaskOutput{Status: TaskStatusFailed, Summary: "Missing"}, wantField: "failure"},
		{
			name: "repair limit with approved review",
			output: TaskOutput{
				Status: TaskStatusRepairLimitReached, Summary: "Inconsistent", Plan: &plan,
				Implementation: &implementation, Validation: &validation, LastReview: &approved,
			},
			wantField: "last_review.approved",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.output.Repository = validRepositoryEvidence()
			assertValidationResult(t, test.output.Validate(), test.wantField)
		})
	}
}
