package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/store"
)

func (service *Service) executeRepair(ctx context.Context, state *runState) *stageFailure {
	const stage = store.WorkflowStageRepair
	attempt := state.repairAttempts + 1
	state.events.stageStarted(stage, attempt)
	if err := state.inspect(ctx, true); err != nil {
		return failureAt(stage, store.FailureCodeWorkspace, err, attempt)
	}

	request := state.repairRequest()
	if err := request.Validate(); err != nil {
		return failureAt(stage, store.FailureCodeInternal, err, attempt)
	}
	implementation, err := invokeAgent(ctx, service, state, stage, func(alternate bool) (store.ImplementationResult, error) {
		if alternate {
			fresh := state.repairRequest()
			fresh.Implementation.AgentSessionID = "" // Sessions never cross provider boundaries.
			return service.fallbacks.Implementer.ApplyReview(ctx, fresh)
		}
		return service.implementer.ApplyReview(ctx, request)
	})
	inspectionErr := state.inspect(ctx, false)
	if err != nil {
		return failureAt(stage, store.FailureCodeAgent, errors.Join(err, inspectionErr), attempt)
	}
	if inspectionErr != nil {
		return failureAt(stage, store.FailureCodeWorkspace, inspectionErr, attempt)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, store.FailureCodeAgent, err, attempt)
	}
	if err := implementation.Validate(); err != nil {
		return failureAt(
			stage,
			store.FailureCodeInvalidOutput,
			fmt.Errorf("invalid repair output: %w", err),
			attempt,
		)
	}

	state.repairAttempts = attempt
	state.setImplementation(implementation)
	state.events.stageCompleted(stage, attempt)
	return nil
}
