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
	if err != nil {
		return failureAt(stage, store.FailureCodeAgent, err, 0)
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

func (service *Service) executeDecidedPlanning(ctx context.Context, state *runState) *stageFailure {
	if service.decisionMaker == nil {
		return service.executePlanning(ctx, state)
	}
	decision, err := service.decisionMaker.DecidePlanning(ctx, state.input)
	if err != nil {
		// Fail open to planning
		return service.executePlanning(ctx, state)
	}
	// Direct implement bypasses planning (threshold already applied in adapter)
	if !decision.NeedsPlanning {
		synth := syntheticPlan(state.input)
		if err := synth.Validate(); err != nil {
			return service.executePlanning(ctx, state)
		}
		// Emit planning stage as completed via decision (no agent invocation)
		if failure := state.beginStage(ctx, store.WorkflowStagePlanning, 0); failure != nil {
			return failure
		}
		state.plan = &synth
		state.events.stageCompleted(store.WorkflowStagePlanning, 0)
		return nil
	}
	return service.executePlanning(ctx, state)
}

func syntheticPlan(input store.TaskInput) store.Plan {
	summary := input.Task
	if len(summary) > 200 {
		summary = summary[:200] + "..."
	}
	return store.Plan{
		Action:             store.PlanActionImplement,
		Summary:            summary,
		Steps:              []string{"Implement task as requested: " + input.Task},
		AcceptanceCriteria: []string{"Task completed per description", "No regressions introduced"},
	}
}

func (service *Service) executeDecidedReview(ctx context.Context, state *runState) *stageFailure {
	if service.decisionMaker == nil {
		return service.executeReview(ctx, state)
	}
	// Validation must have passed to allow auto-approve
	decision, err := service.decisionMaker.DecideReview(ctx, state.reviewRequest())
	if err != nil {
		return service.executeReview(ctx, state)
	}
	if !decision.ShouldReview {
		if !state.validation.Passed {
			return service.executeReview(ctx, state)
		}
		if failure := state.beginStage(ctx, store.WorkflowStageReview, state.repairAttempts); failure != nil {
			return failure
		}
		if err := state.inspect(ctx, true); err != nil {
			return failureAt(store.WorkflowStageReview, store.FailureCodeWorkspace, err, state.repairAttempts)
		}
		synth := store.Review{
			Approved: decision.Approved,
			Summary:  "Auto-approved by Jev decision model: " + decision.Reason,
			Findings: nil,
		}
		if !synth.Approved {
			synth.Findings = []store.ReviewFinding{{
				Severity:       store.FindingSeverityWarning,
				Blocking:       true,
				Description:    "Jev flagged for review",
				RequiredAction: "Route to full review",
			}}
			// If Jev says not approved but we bypassed ShouldReview, treat as needs review
			return service.executeReview(ctx, state)
		}
		if err := synth.Validate(); err != nil {
			return service.executeReview(ctx, state)
		}
		state.review = &synth
		state.events.stageCompleted(store.WorkflowStageReview, state.repairAttempts)
		return nil
	}
	return service.executeReview(ctx, state)
}

func (service *Service) executeInitialImplementation(
	ctx context.Context,
	state *runState,
) *stageFailure {
	const stage = store.WorkflowStageImplementation
	if failure := state.beginStage(ctx, stage, 0); failure != nil {
		return failure
	}
	// Only an implementation plan needs a baseline. Capture current user work
	// before any mutating agent runs; planning relies on read-only provider policy.
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
		state.repairAttempts = attempt
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

	state.setImplementation(implementation)
	state.events.stageCompleted(stage, attempt)
	return nil
}
