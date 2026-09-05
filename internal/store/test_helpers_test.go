package store

import (
	"errors"
	"testing"
)

func assertValidationResult(t *testing.T, err error, expectedField string) {
	t.Helper()
	if expectedField == "" {
		if err != nil {
			t.Fatalf("Validate() returned an error: %v", err)
		}
		return
	}

	if err == nil {
		t.Fatalf("Validate() returned nil; want an error for %s", expectedField)
	}

	var validationErr *ContractValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("error type = %T; want *ContractValidationError", err)
	}
	if validationErr.Field != expectedField {
		t.Fatalf("validation field = %q; want %q (error: %v)", validationErr.Field, expectedField, err)
	}
}

func validTaskInput() TaskInput {
	return TaskInput{
		Task:              "add health endpoint",
		WorkingDir:        "/workspace/project",
		MaxRepairAttempts: 2,
	}
}

func validPlan() Plan {
	return Plan{
		Action:             PlanActionImplement,
		Summary:            "Add a health endpoint",
		Steps:              []string{"add the handler", "register the route"},
		AcceptanceCriteria: []string{"GET /health returns HTTP 200"},
	}
}

func validImplementationResult() ImplementationResult {
	return ImplementationResult{
		Summary:        "Added and tested the health endpoint",
		ChangedFiles:   []string{"server.go", "server_test.go"},
		AgentSessionID: "session-123",
	}
}

func validValidationReport() ValidationReport {
	return ValidationReport{
		Passed: true,
		Checks: []ValidationEvidence{
			{
				Command:        "go test ./...",
				Passed:         true,
				ExitCode:       0,
				Output:         "ok multiharness",
				DurationMillis: 125,
			},
		},
	}
}

func validApprovedReview() Review {
	return Review{
		Approved: true,
		Summary:  "All acceptance criteria are satisfied",
		Findings: []ReviewFinding{
			{
				Severity:    FindingSeverityInfo,
				Description: "The handler follows the existing response pattern",
			},
		},
		Suggestions: []string{"Consider adding latency monitoring later"},
	}
}

func validRejectedReview() Review {
	return Review{
		Approved: false,
		Summary:  "One blocking issue remains",
		Findings: []ReviewFinding{
			{
				Severity:       FindingSeverityError,
				Blocking:       true,
				File:           "server.go",
				Line:           42,
				Description:    "The error response has no body",
				Evidence:       "The handler returns immediately after WriteHeader",
				RequiredAction: "Encode the documented error body before returning",
			},
		},
	}
}
