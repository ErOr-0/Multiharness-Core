package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/store"
)

func (service *Service) executeInitialImplementation(
	ctx context.Context,
	state *runState,
) *stageFailure {
	const stage = store.WorkflowStageImplementation
	state.events.stageStarted(stage, 0)

	request := state.implementationRequest()
	if err := request.Validate(); err != nil {
		return failureAt(stage, store.FailureCodeInternal, err, 0)
	}
	implementation, err := invokeAgent(ctx, service, state, stage, func(alternate bool) (store.ImplementationResult, error) {
		if alternate {
			return service.fallbacks.Implementer.Implement(ctx, state.implementationRequest())
		}
		return service.implementer.Implement(ctx, request)
	})
	inspectionErr := state.inspect(ctx, false)
	if err != nil {
		return failureAt(stage, store.FailureCodeAgent, errors.Join(err, inspectionErr), 0)
	}
	if inspectionErr != nil {
		return failureAt(stage, store.FailureCodeWorkspace, inspectionErr, 0)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, store.FailureCodeAgent, err, 0)
	}
	if err := implementation.Validate(); err != nil {
		return failureAt(
			stage,
			store.FailureCodeInvalidOutput,
			fmt.Errorf("invalid implementer output: %w", err),
			0,
		)
	}

	state.setImplementation(implementation)
	state.events.stageCompleted(stage, 0)
	return nil
}
