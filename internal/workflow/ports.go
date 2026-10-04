package workflow

import (
	"context"
	"time"

	"multiharness-core/internal/contract"
)

// RetryWaiter is context-aware and injectable for deterministic policy tests.
type RetryWaiter interface {
	Wait(context.Context, time.Duration) error
}

// PermissionResolver waits for the user to resolve a native access block. A true
// response authorizes another invocation, never a broader permission policy.
type PermissionResolver interface {
	ResolvePermission(context.Context, contract.WorkflowStage, contract.PermissionDenied) (bool, error)
}

// Planner produces a structured plan for the original task. Agent ports return
// errors wrapping *contract.ProviderFailure for recognized provider errors; context
// cancellation remains inspectable with errors.Is. Raw diagnostics stay outside
// the public failure contract. The same error convention applies to Implementer
// and Reviewer. Planning must use provider-enforced read-only permissions;
// no workspace baseline exists during planning.
type Planner interface {
	Plan(ctx context.Context, input contract.TaskInput) (contract.Plan, error)
}

// Workspace checks readiness and acquires exclusive access in one operation.
// Concrete filesystem or remote-workspace behavior belongs to an outer adapter.
type Workspace interface {
	Acquire(ctx context.Context, workingDir string) (WorkspaceSession, error)
}

// WorkspaceSession holds exclusive access from implementation through completion. Inspect returns
// baseline-relative evidence, even on error when possible. Close releases the
// lease without resetting, staging, or otherwise modifying user files.
type WorkspaceSession interface {
	Baseline() contract.RepositoryEvidence
	Inspect(context.Context) (contract.RepositoryEvidence, error)
	Close() error // Idempotent, and independent of the cancelled run context.
}

// Implementer performs the initial implementation and any later repairs.
// Handoffs are bounded projections: full workspace evidence stays in workflow
// state while prompts carry only task, plan, workspace identity, manifests and
// blocking evidence. Implementations fail locally before provider execution
// when a compact projection still exceeds execution.max_prompt_bytes.
type Implementer interface {
	Implement(
		ctx context.Context,
		request contract.ImplementationRequest,
	) (contract.ImplementationResult, error)

	ApplyReview(
		ctx context.Context,
		request contract.RepairRequest,
	) (contract.ImplementationResult, error)
}

// Validator runs deterministic checks independently from the implementation
// agent and returns inspectable evidence.
type Validator interface {
	Validate(ctx context.Context, request contract.ValidationRequest) (contract.ValidationReport, error)
}

// Reviewer inspects a cohesive request containing the original task, plan,
// implementation result, and independent validation evidence.
// Oversized evidence is partitioned by file and hunk into bounded ReviewChunks
// (execution.review_chunk_bytes); approval requires every chunk reviewed,
// validation passed and an unchanged workspace fingerprint.
type Reviewer interface {
	Review(ctx context.Context, request contract.ReviewRequest) (contract.Review, error)
}

// BatchReviewer tells the reviewer which chunk of how many it is reviewing and
// adds one synthesis call over collected findings. Reviewers without it
// receive each chunk as an ordinary scoped Review request and no synthesis.
type BatchReviewer interface {
	Reviewer
	ReviewChunk(ctx context.Context, request contract.ReviewRequest, chunk contract.ReviewChunk) (contract.Review, error)
	ReviewSynthesis(ctx context.Context, request contract.ReviewRequest, findings []contract.ReviewFinding, chunkSummaries []string) (contract.Review, error)
}

// DecisionMaker routes planning/review via Jev System One (OpenRouter).
// Implementations return an explicit intent or a conservative assessment fallback.
type DecisionMaker interface {
	DecidePlanning(ctx context.Context, input contract.TaskInput) (contract.PlanningDecision, error)
	DecideReview(ctx context.Context, request contract.ReviewRequest) (contract.ReviewDecision, error)
}

// WorkspaceApprover grants permission to update backed-up existing files before
// implementation. Absence, refusal or cancellation must never imply approval.
type WorkspaceApprover interface {
	ConfirmExistingWork(context.Context, contract.ExistingWork) (bool, error)
}
