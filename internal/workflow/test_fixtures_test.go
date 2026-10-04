package workflow_test

import "multiharness-core/internal/contract"

func validTask(maxRepairAttempts int) contract.TaskInput {
	return contract.TaskInput{
		Task:              "implement the requested change",
		WorkingDir:        "/workspace/project",
		MaxRepairAttempts: maxRepairAttempts,
	}
}

func validPlan() contract.Plan {
	return contract.Plan{
		Action:             contract.PlanActionImplement,
		Summary:            "Implement and verify the change",
		HandoffContext:     []string{"service.go owns the workflow transition"},
		Steps:              []string{"update the implementation", "run deterministic checks"},
		AcceptanceCriteria: []string{"the requested behavior is covered by tests"},
	}
}

func implementation(summary, changedFile string) contract.ImplementationResult {
	return contract.ImplementationResult{
		Summary:        summary,
		ChangedFiles:   []string{changedFile},
		AgentSessionID: "session-123",
	}
}

func passingValidation() contract.ValidationReport {
	return contract.ValidationReport{
		Passed: true,
		Checks: []contract.ValidationEvidence{{
			Command:        "go test ./...",
			Passed:         true,
			ExitCode:       0,
			Output:         "ok",
			DurationMillis: 10,
		}},
	}
}

func failingValidation() contract.ValidationReport {
	return contract.ValidationReport{
		Passed: false,
		Checks: []contract.ValidationEvidence{{
			Command:        "go test ./...",
			Passed:         false,
			ExitCode:       1,
			Output:         "test failed",
			DurationMillis: 10,
		}},
	}
}

func approvedReview(summary string) contract.Review {
	return contract.Review{Approved: true, Summary: summary}
}

func rejectedReview(summary string) contract.Review {
	return contract.Review{
		Approved: false,
		Summary:  summary,
		Findings: []contract.ReviewFinding{{
			Severity:       contract.FindingSeverityError,
			Blocking:       true,
			File:           "service.go",
			Line:           12,
			Description:    "the edge case is not handled",
			Evidence:       "the failing branch returns the wrong status",
			RequiredAction: "handle the edge case and add a regression test",
		}},
	}
}
