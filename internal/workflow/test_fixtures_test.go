package workflow_test

import "multiharness-core/internal/store"

func validTask(maxRepairAttempts int) store.TaskInput {
	return store.TaskInput{
		Task:              "implement the requested change",
		WorkingDir:        "/workspace/project",
		MaxRepairAttempts: maxRepairAttempts,
	}
}

func validPlan() store.Plan {
	return store.Plan{
		Action:             store.PlanActionImplement,
		Summary:            "Implement and verify the change",
		Steps:              []string{"update the implementation", "run deterministic checks"},
		AcceptanceCriteria: []string{"the requested behavior is covered by tests"},
	}
}

func implementation(summary, changedFile string) store.ImplementationResult {
	return store.ImplementationResult{
		Summary:        summary,
		ChangedFiles:   []string{changedFile},
		AgentSessionID: "session-123",
	}
}

func passingValidation() store.ValidationReport {
	return store.ValidationReport{
		Passed: true,
		Checks: []store.ValidationEvidence{{
			Command:        "go test ./...",
			Passed:         true,
			ExitCode:       0,
			Output:         "ok",
			DurationMillis: 10,
		}},
	}
}

func failingValidation() store.ValidationReport {
	return store.ValidationReport{
		Passed: false,
		Checks: []store.ValidationEvidence{{
			Command:        "go test ./...",
			Passed:         false,
			ExitCode:       1,
			Output:         "test failed",
			DurationMillis: 10,
		}},
	}
}

func approvedReview(summary string) store.Review {
	return store.Review{Approved: true, Summary: summary}
}

func rejectedReview(summary string) store.Review {
	return store.Review{
		Approved: false,
		Summary:  summary,
		Findings: []store.ReviewFinding{{
			Severity:       store.FindingSeverityError,
			Blocking:       true,
			File:           "service.go",
			Line:           12,
			Description:    "the edge case is not handled",
			Evidence:       "the failing branch returns the wrong status",
			RequiredAction: "handle the edge case and add a regression test",
		}},
	}
}
