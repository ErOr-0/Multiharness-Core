package schemaexec

import (
	"context"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// Reviewer invokes Codex in a fresh, schema-constrained review role.
type Reviewer struct {
	executor executor
}

// NewReviewer constructs a Codex workflow reviewer with validated configuration.
func NewReviewer(runner ProcessRunner, config Config) (*Reviewer, error) {
	executor, err := newExecutor(runner, config)
	if err != nil {
		return nil, err
	}
	return &Reviewer{executor: executor}, nil
}

// Review independently inspects the workspace and validates its structured decision.
func (reviewer *Reviewer) Review(
	ctx context.Context,
	request store.ReviewRequest,
) (store.Review, error) {
	if err := request.Validate(); err != nil {
		return store.Review{}, err
	}
	prompt, err := structured.ReviewPrompt(request)
	if err != nil {
		return store.Review{}, err
	}
	output, err := reviewer.executor.execute(
		ctx,
		roleReview,
		request.Input.WorkingDir,
		structured.ReviewSchema(),
		prompt,
	)
	if err != nil {
		return store.Review{}, err
	}
	review, err := structured.ParseReview(output)
	if err != nil {
		return review, &OutputError{Role: roleReview, Cause: err}
	}
	return review, nil
}

var _ workflow.Reviewer = (*Reviewer)(nil)
