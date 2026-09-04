package workflow

import (
	"context"
	"errors"

	"multiharness-core/internal/store"
)

// Run is the typed entry point for one task, with no framework initialization
// required. It validates input, executes stages using the caller's context, and
// reports success, failure, and cancellation through a structured TaskOutput.
// Each stage owns its execution details and records evidence in the per-run state.
func (service *Service) Run(ctx context.Context, input store.TaskInput) store.TaskOutput {
	state := newRunState(input, service.events)
	// A panic in an injected port must not strand the exclusive workspace lease.
	defer func() {
		if state.workspace != nil {
			_ = state.workspace.Close()
		}
	}()
	failure := service.runStages(ctx, state)
	if state.workspace != nil {
		if err := state.workspace.Close(); err != nil {
			if failure == nil {
				stage := store.WorkflowStageReview
				if state.plan.Action == store.PlanActionAnswer {
					stage = store.WorkflowStagePlanning
				}
				failure = failureAt(stage, store.FailureCodeWorkspace, err, state.repairAttempts)
			} else {
				failure.cause = errors.Join(failure.cause, err)
			}
		}
	}
	if failure != nil {
		return state.terminalFrom(ctx, failure)
	}
	if state.plan.Action == store.PlanActionAnswer {
		return state.answered()
	}
	if state.review.Approved {
		return state.approved()
	}
	return state.repairLimitReached()
}

func (service *Service) runStages(ctx context.Context, state *runState) *stageFailure {
	if failure := service.executeIntake(ctx, state); failure != nil {
		return failure
	}
	if failure := service.executePlanning(ctx, state); failure != nil {
		return failure
	}
	if state.plan.Action == store.PlanActionAnswer {
		return nil
	}
	if failure := service.executeInitialImplementation(ctx, state); failure != nil {
		return failure
	}

	for {
		if failure := service.executeValidation(ctx, state); failure != nil {
			return failure
		}
		if failure := service.executeReview(ctx, state); failure != nil {
			return failure
		}

		if state.review.Approved {
			return nil
		}
		if !state.input.RepairAvailable(state.repairAttempts) {
			return nil
		}
		if failure := service.executeRepair(ctx, state); failure != nil {
			return failure
		}
	}
}
