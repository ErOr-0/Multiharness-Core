package workflow

import (
	"context"
	"fmt"

	"multiharness-core/internal/store"
)

// Run is the typed entry point for one task, with no framework initialization
// required. It validates input, executes stages using the caller's context, and
// reports success, failure, and cancellation through a structured TaskOutput.
// Stage details are in stages.go, in the same order as runStages below.
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

	// Every implementation, including repairs, must pass validation and review.
	// Exhaustion returns the last rejection, never approval.
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

// Dependencies contains the required outbound ports for Service.
type Dependencies struct {
	Workspace   Workspace
	Planner     Planner
	Implementer Implementer
	Validator   Validator
	Reviewer    Reviewer
	Events      EventSink
	Execution   ExecutionPolicy
	RetryWaiter RetryWaiter
	Fallbacks   BillingFallbacks
}

// DependencyError identifies a missing required Service dependency.
type DependencyError struct {
	Name string
}

func (err *DependencyError) Error() string {
	return fmt.Sprintf("workflow dependency %q is required", err.Name)
}

// Service coordinates one workflow run using injected ports. It is safe for
// concurrent use when its injected dependencies are safe for concurrent use.
type Service struct {
	workspace   Workspace
	planner     Planner
	implementer Implementer
	validator   Validator
	reviewer    Reviewer
	events      EventSink
	execution   ExecutionPolicy
	retryWaiter RetryWaiter
	fallbacks   BillingFallbacks
}

// NewService validates dependencies and returns an immutable workflow service.
func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Workspace == nil {
		return nil, &DependencyError{Name: "workspace"}
	}
	if dependencies.Planner == nil {
		return nil, &DependencyError{Name: "planner"}
	}
	if dependencies.Implementer == nil {
		return nil, &DependencyError{Name: "implementer"}
	}
	if dependencies.Validator == nil {
		return nil, &DependencyError{Name: "validator"}
	}
	if dependencies.Reviewer == nil {
		return nil, &DependencyError{Name: "reviewer"}
	}

	if err := dependencies.Fallbacks.validate(); err != nil {
		return nil, err
	}
	execution := dependencies.Execution.withDefaults()
	if err := execution.Validate(); err != nil {
		return nil, err
	}
	waiter := dependencies.RetryWaiter
	if waiter == nil {
		waiter = timerWaiter{}
	}
	return &Service{
		workspace:   dependencies.Workspace,
		planner:     dependencies.Planner,
		implementer: dependencies.Implementer,
		validator:   dependencies.Validator,
		reviewer:    dependencies.Reviewer,
		events:      dependencies.Events,
		execution:   execution,
		retryWaiter: waiter,
		fallbacks:   dependencies.Fallbacks,
	}, nil
}
