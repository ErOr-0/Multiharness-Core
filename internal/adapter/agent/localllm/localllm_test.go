package localllm_test

import (
	"context"
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
	plan, err := strategy.Plan(ctx, store.TaskInput{
		Task:       "Add a greeting function",
		WorkingDir: "/test/dir",
	})
	if err != nil {
		t.Fatalf("planning failed: %v", err)
	}
	if plan.Action != store.PlanActionImplement {
		t.Errorf("expected plan action implement, got %v", plan.Action)
	}

	// 2. Test Implementation
	impl, err := strategy.Implement(ctx, store.ImplementationRequest{
		Input: store.TaskInput{Task: "Add greeting", WorkingDir: "/test/dir"},
		Plan:  plan,
	})
	if err != nil {
		t.Fatalf("implementation failed: %v", err)
	}
	if impl.Summary == "" {
		t.Errorf("expected non-empty summary")
	}

	// 3. Test Review
	review, err := strategy.Review(ctx, store.ReviewRequest{
		Input:          store.TaskInput{Task: "Add greeting", WorkingDir: "/test/dir"},
		Plan:           plan,
		Implementation: impl,
		Validation: store.ValidationReport{
			Passed: true,
		},
	})
	if err != nil {
		t.Fatalf("review failed: %v", err)
	}
	if !review.Approved {
		t.Errorf("expected approved review when validation passed")
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
	if err != nil {
		t.Fatalf("repair failed: %v", err)
	}
	if repair.Summary == "" {
		t.Errorf("expected non-empty repair summary")
	}
}
