package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/store"
)

var errNilContext = errors.New("workflow context must not be nil")

func (service *Service) executeIntake(ctx context.Context, state *runState) *stageFailure {
	const stage = store.WorkflowStageIntake
	if failure := state.beginStage(ctx, stage, 0); failure != nil {
		return failure
	}
	if err := state.input.Validate(); err != nil {
		return failureAt(stage, store.FailureCodeInvalidInput, err, 0)
	}
	// Structural input errors are invalid_input; runtime readiness, locking and
	// capture failures are workspace_error. Acquire owns all workspace preflight.
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

func (service *Service) executeInitialImplementation(
	ctx context.Context,
	state *runState,
) *stageFailure {
	const stage = store.WorkflowStageImplementation
	if failure := state.beginStage(ctx, stage, 0); failure != nil {
		return failure
	}
	if err := state.inspect(ctx, true); err != nil {
		return failureAt(stage, store.FailureCodeWorkspace, err, 0)
	}

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

func (service *Service) executeReview(ctx context.Context, state *runState) *stageFailure {
	const stage = store.WorkflowStageReview
	attempt := state.repairAttempts
	if failure := state.beginStage(ctx, stage, attempt); failure != nil {
		return failure
	}
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

	state.review = &review
	if !review.Approved {
		state.events.stageProgress(stage, attempt, state.blockingFindingCount())
	}
	state.events.stageCompleted(stage, attempt)
	return nil
}

func (service *Service) executeRepair(ctx context.Context, state *runState) *stageFailure {
	const stage = store.WorkflowStageRepair
	attempt := state.repairAttempts + 1
	if failure := state.beginStage(ctx, stage, attempt); failure != nil {
		return failure
	}
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
