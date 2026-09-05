package workflow

import (
	"context"

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
	failure = state.releaseWorkspace(failure)
	if failure != nil {
		return state.terminalFrom(ctx, failure)
	}
	// Completion callbacks and lease cleanup can cancel the run after its last
	// stage returned. Resolve cancellation before publishing a terminal outcome.
	if err := ctx.Err(); err != nil {
		stage := store.WorkflowStageReview
		if state.plan.Action == store.PlanActionAnswer {
			stage = store.WorkflowStagePlanning
		}
		return state.cancelled(stage, err, state.repairAttempts)
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
	return service.reviewUntilApproved(ctx, state)
}

// Every implementation, including repairs, must pass the same validation and
// independent review. Exhaustion returns the last rejection, never approval.
func (service *Service) reviewUntilApproved(ctx context.Context, state *runState) *stageFailure {
	for {
		if failure := service.executeValidation(ctx, state); failure != nil {
			return failure
		}
		if failure := service.executeReview(ctx, state); failure != nil {
			return failure
		}
		if state.review.Approved || !state.input.RepairAvailable(state.repairAttempts) {
			return nil
		}
		if failure := service.executeRepair(ctx, state); failure != nil {
			return failure
		}
	}
}
