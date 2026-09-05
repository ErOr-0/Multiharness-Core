package workflow

import (
	"context"
	"time"

	"multiharness-core/internal/store"
)

// RetryWaiter is context-aware and injectable for deterministic policy tests.
type RetryWaiter interface {
	Wait(context.Context, time.Duration) error
}

// Planner produces a structured plan for the original task. Agent ports return
// errors wrapping *store.ProviderFailure for recognized provider errors; context
// cancellation remains inspectable with errors.Is. Raw diagnostics stay outside
// the public failure contract. The same error convention applies to Implementer
// and Reviewer.
type Planner interface {
	Plan(ctx context.Context, input store.TaskInput) (store.Plan, error)
}

// Workspace checks readiness and acquires exclusive access in one operation.
// Concrete filesystem or remote-workspace behavior belongs to an outer adapter.
type Workspace interface {
	Acquire(ctx context.Context, workingDir string) (WorkspaceSession, error)
}

// WorkspaceSession holds exclusive access for the entire run. Inspect returns
// baseline-relative evidence, even on error when possible. Close releases the
// lease without resetting, staging, or otherwise modifying user files.
type WorkspaceSession interface {
	Baseline() store.RepositoryEvidence
	Inspect(context.Context) (store.RepositoryEvidence, error)
	Close() error // Idempotent, and independent of the cancelled run context.
}

// Implementer performs the initial implementation and any later repairs.
type Implementer interface {
	Implement(
		ctx context.Context,
		request store.ImplementationRequest,
	) (store.ImplementationResult, error)

	ApplyReview(
		ctx context.Context,
		request store.RepairRequest,
	) (store.ImplementationResult, error)
}

// Validator runs deterministic checks independently from the implementation
// agent and returns inspectable evidence.
type Validator interface {
	Validate(ctx context.Context, request store.ValidationRequest) (store.ValidationReport, error)
}

// Reviewer inspects a cohesive request containing the original task, plan,
// implementation result, and independent validation evidence.
type Reviewer interface {
	Review(ctx context.Context, request store.ReviewRequest) (store.Review, error)
}
