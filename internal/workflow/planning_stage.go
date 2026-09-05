package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/store"
)

func (service *Service) executePlanning(ctx context.Context, state *runState) *stageFailure {
	const stage = store.WorkflowStagePlanning
	if failure := state.beginStage(ctx, stage, 0); failure != nil {
		return failure
	}

	plan, err := invokeAgent(ctx, service, state, stage, func(alternate bool) (store.Plan, error) {
		if alternate {
			return service.fallbacks.Planner.Plan(ctx, state.input)
		}
		return service.planner.Plan(ctx, state.input)
	})
	inspectionErr := state.inspect(ctx, true)
	if err != nil {
		return failureAt(stage, store.FailureCodeAgent, errors.Join(err, inspectionErr), 0)
	}
	if inspectionErr != nil {
		return failureAt(stage, store.FailureCodeWorkspace, inspectionErr, 0)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, store.FailureCodeAgent, err, 0)
	}
	if err := plan.Validate(); err != nil {
		return failureAt(
			stage,
			store.FailureCodeInvalidOutput,
			fmt.Errorf("invalid planner output: %w", err),
			0,
		)
	}

	state.plan = &plan
	state.events.stageCompleted(stage, 0)
	return nil
}
