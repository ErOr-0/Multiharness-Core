package workflow

import (
	"context"
	"errors"

	"multiharness-core/internal/store"
)

var errNilContext = errors.New("workflow context must not be nil")

func (service *Service) executeIntake(ctx context.Context, state *runState) *stageFailure {
	const stage = store.WorkflowStageIntake
	state.events.stageStarted(stage, 0)

	if ctx == nil {
		return failureAt(stage, store.FailureCodeInternal, errNilContext, 0)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, store.FailureCodeInternal, err, 0)
	}
	if err := state.input.Validate(); err != nil {
		return failureAt(stage, store.FailureCodeInvalidInput, err, 0)
	}
	if err := service.workspace.Validate(ctx, state.input.WorkingDir); err != nil {
		return failureAt(stage, store.FailureCodeInvalidInput, err, 0)
	}
	lease, err := service.workspace.Acquire(ctx, state.input.WorkingDir)
	if err != nil {
		return failureAt(stage, store.FailureCodeWorkspace, err, 0)
	}
	if lease == nil {
		return failureAt(stage, store.FailureCodeInternal, errors.New("workspace returned a nil session"), 0)
	}
	state.workspace = lease
	baseline := lease.Baseline()
	if err := baseline.Validate(); err != nil {
		return failureAt(stage, store.FailureCodeWorkspace, err, 0)
	}
	if baseline.Baseline != baseline.Current || len(baseline.ChangedFiles) != 0 {
		return failureAt(stage, store.FailureCodeWorkspace, errors.New("workspace baseline already contains run changes"), 0)
	}
	state.repository = baseline.Clone()
	if err := state.checkRepository(); err != nil {
		return failureAt(stage, store.FailureCodeWorkspace, err, 0)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, store.FailureCodeInternal, err, 0)
	}

	state.events.stageCompleted(stage, 0)
	return nil
}
