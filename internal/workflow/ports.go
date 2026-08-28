package workflow

import "context"

type Planner interface {
	Plan(ctx context.Context, input TaskInput) (Plan, error)
}

type Implementer interface {
	Implement(
		ctx context.Context,
		input TaskInput,
		plan Plan,
	) error

	ApplyReview(
		ctx context.Context,
		input TaskInput,
		review Review,
	) error
}

type Reviewer interface {
	Review(
		ctx context.Context,
		input TaskInput,
		plan Plan,
	) (Review, error)
}
