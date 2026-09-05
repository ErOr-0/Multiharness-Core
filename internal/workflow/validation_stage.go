package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/store"
)

func (service *Service) executeValidation(ctx context.Context, state *runState) *stageFailure {
	const stage = store.WorkflowStageValidation
	attempt := state.repairAttempts
	if failure := state.beginStage(ctx, stage, attempt); failure != nil {
		return failure
	}
	if err := state.inspect(ctx, true); err != nil {
		return failureAt(stage, store.FailureCodeWorkspace, err, attempt)
	}

	request := state.validationRequest()
	if err := request.Validate(); err != nil {
		return failureAt(stage, store.FailureCodeInternal, err, attempt)
	}
	validation, err := service.validator.Validate(ctx, request)
	// Retain completed command evidence even when a later command failed.
	validationErr := validation.Validate()
	if validationErr == nil {
		state.setValidation(validation)
	}
	inspectionErr := state.inspect(ctx, true)
	if err != nil {
		return failureAt(stage, store.FailureCodeValidation, errors.Join(err, inspectionErr), attempt)
	}
	if inspectionErr != nil {
		return failureAt(stage, store.FailureCodeWorkspace, inspectionErr, attempt)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, store.FailureCodeValidation, err, attempt)
	}
	if validationErr != nil {
		return failureAt(
			stage,
			store.FailureCodeInvalidOutput,
			fmt.Errorf("invalid validator output: %w", validationErr),
			attempt,
		)
	}

	state.events.stageCompleted(stage, attempt)
	return nil
}
