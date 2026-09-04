package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/store"
)

func (service *Service) executeReview(ctx context.Context, state *runState) *stageFailure {
	const stage = store.WorkflowStageReview
	attempt := state.repairAttempts
	state.events.stageStarted(stage, attempt)
	if err := state.inspect(ctx, true); err != nil {
		return failureAt(stage, store.FailureCodeWorkspace, err, attempt)
	}

	request := state.reviewRequest()
	if err := request.Validate(); err != nil {
		return failureAt(stage, store.FailureCodeInternal, err, attempt)
	}
	review, err := invokeAgent(ctx, service, state, stage, func(alternate bool) (store.Review, error) {
		if alternate {
			return service.fallbacks.Reviewer.Review(ctx, state.reviewRequest())
		}
		return service.reviewer.Review(ctx, request)
	})
	inspectionErr := state.inspect(ctx, true)
	if err != nil {
		return failureAt(stage, store.FailureCodeAgent, errors.Join(err, inspectionErr), attempt)
	}
	if inspectionErr != nil {
		return failureAt(stage, store.FailureCodeWorkspace, inspectionErr, attempt)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, store.FailureCodeAgent, err, attempt)
	}
	if err := review.Validate(); err != nil {
		return failureAt(
			stage,
			store.FailureCodeInvalidOutput,
			fmt.Errorf("invalid reviewer output: %w", err),
			attempt,
		)
	}
	if review.Approved && !state.validation.Passed {
		return failureAt(
			stage,
			store.FailureCodeInvalidOutput,
			errors.New("reviewer approved an implementation with failed deterministic validation"),
			attempt,
		)
	}

	state.setReview(review)
	if !review.Approved {
		state.events.stageProgress(stage, attempt, state.blockingFindingCount())
	}
	state.events.stageCompleted(stage, attempt)
	return nil
}
