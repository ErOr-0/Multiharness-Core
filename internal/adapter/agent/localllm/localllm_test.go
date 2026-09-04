package localllm_test

import (
	"context"
	"errors"
	"testing"

	"multiharness-core/internal/adapter/agent/localllm"
	"multiharness-core/internal/store"
)

func TestLocalLLMStrategy(t *testing.T) {
	strategy, err := localllm.NewLocalLLMStrategy(localllm.Config{
		Model: "llama3:8b",
	})
	if err != nil {
		t.Fatalf("failed to create strategy: %v", err)
	}

	ctx := context.Background()

	// 1. Test Planning
	_, err = strategy.Plan(ctx, store.TaskInput{
		Task:       "Add a greeting function",
		WorkingDir: "/test/dir",
	})
	if !errors.Is(err, localllm.ErrUnavailable) {
		t.Fatalf("unfinished planner returned %v", err)
	}
	plan := store.Plan{Action: store.PlanActionImplement, Summary: "add tests", Steps: []string{"add tests"}, AcceptanceCriteria: []string{"tests pass"}}

	// 2. Test Implementation
	impl, err := strategy.Implement(ctx, store.ImplementationRequest{
		Input: store.TaskInput{Task: "Add greeting", WorkingDir: "/test/dir"},
		Plan:  plan,
	})
	if !errors.Is(err, localllm.ErrUnavailable) || impl.Summary != "" {
		t.Fatal("unfinished implementer manufactured success")
	}
	impl = store.ImplementationResult{Summary: "claimed work"}

	// 3. Test Review
	review, err := strategy.Review(ctx, store.ReviewRequest{
		Input:          store.TaskInput{Task: "Add greeting", WorkingDir: "/test/dir"},
		Plan:           plan,
		Implementation: impl,
		Validation: store.ValidationReport{
			Passed: true,
		},
	})
	if !errors.Is(err, localllm.ErrUnavailable) || review.Approved {
		t.Fatal("unfinished reviewer manufactured approval")
	}

	// 4. Test Repair with a rejected review and failed validation
	failedValidation := store.ValidationReport{
		Passed: false,
		Checks: []store.ValidationEvidence{
			{Command: "go test", Passed: false, ExitCode: 1, DurationMillis: 10},
		},
	}
	rejectedReview := store.Review{
		Approved: false,
		Summary:  "Test failure needs repair",
		Findings: []store.ReviewFinding{
			{
				Severity:       store.FindingSeverityError,
				Blocking:       true,
				Description:    "Unit tests failed",
				RequiredAction: "Fix broken test",
			},
		},
	}
	repair, err := strategy.ApplyReview(ctx, store.RepairRequest{
		Input:          store.TaskInput{Task: "Add greeting", WorkingDir: "/test/dir"},
		Plan:           plan,
		Implementation: impl,
		Validation:     failedValidation,
		Review:         rejectedReview,
	})
	if !errors.Is(err, localllm.ErrUnavailable) || repair.Summary != "" {
		t.Fatal("unfinished repair manufactured success")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = strategy.Plan(cancelled, store.TaskInput{Task: "add tests", WorkingDir: "/test/dir"})
	if !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation")
	}
}
