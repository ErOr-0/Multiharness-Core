package workflow

import (
	"context"

	"multiharness-core/internal/store"
)

type contractAgent struct{}

func (contractAgent) Plan(context.Context, store.TaskInput) (store.Plan, error) {
	return store.Plan{}, nil
}

func (contractAgent) Implement(context.Context, store.ImplementationRequest) (store.ImplementationResult, error) {
	return store.ImplementationResult{}, nil
}

func (contractAgent) ApplyReview(context.Context, store.RepairRequest) (store.ImplementationResult, error) {
	return store.ImplementationResult{}, nil
}

func (contractAgent) Review(context.Context, store.ReviewRequest) (store.Review, error) {
	return store.Review{}, nil
}

type contractValidator struct{}

func (contractValidator) Validate(context.Context, store.ValidationRequest) (store.ValidationReport, error) {
	return store.ValidationReport{}, nil
}

type contractWorkspace struct{}

func (contractWorkspace) Acquire(context.Context, string) (WorkspaceSession, error) { return nil, nil }

func (contractWorkspace) Validate(context.Context, string) error {
	return nil
}

var (
	_ Planner     = contractAgent{}
	_ Implementer = contractAgent{}
	_ Reviewer    = contractAgent{}
	_ Validator   = contractValidator{}
	_ Workspace   = contractWorkspace{}
)
