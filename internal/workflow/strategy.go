package workflow

import (
	"context"

	"multiharness-core/internal/store"
)

// PlanningStrategy provides a pluggable strategy interface for planning and question-answering.
// Implementations can wrap Codex, OpenCode, or local LLM providers without altering
// workflow orchestration.
type PlanningStrategy interface {
	Plan(ctx context.Context, input store.TaskInput) (store.Plan, error)
}

// ImplementationStrategy provides a pluggable strategy interface for initial code generation
// and subsequent review repairs.
type ImplementationStrategy interface {
	Implement(ctx context.Context, request store.ImplementationRequest) (store.ImplementationResult, error)
	ApplyReview(ctx context.Context, request store.RepairRequest) (store.ImplementationResult, error)
}

// ReviewStrategy provides a pluggable strategy interface for independent code inspection
// against plans and deterministic validation evidence.
type ReviewStrategy interface {
	Review(ctx context.Context, request store.ReviewRequest) (store.Review, error)
}

// ModelStrategyRegistry allows registering and swapping model execution strategies
// dynamically or through configuration.
type ModelStrategyRegistry struct {
	Planner     PlanningStrategy
	Implementer ImplementationStrategy
	Reviewer    Reviewer
}

// DefaultModelStrategies returns the configured primary strategies for the workflow.
func (s *Service) DefaultModelStrategies() ModelStrategyRegistry {
	return ModelStrategyRegistry{
		Planner:     s.planner,
		Implementer: s.implementer,
		Reviewer:    s.reviewer,
	}
}
