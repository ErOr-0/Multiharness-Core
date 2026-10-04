package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/contract"
)

var errNilContext = errors.New("workflow context must not be nil")

func (service *Service) executeIntake(ctx context.Context, state *runState) *stageFailure {
	const stage = contract.WorkflowStageIntake
	if failure := state.beginStage(ctx, stage, 0); failure != nil {
		return failure
	}
	if err := state.input.Validate(); err != nil {
		return failureAt(stage, contract.FailureCodeInvalidInput, err, 0)
	}

	state.events.stageCompleted(stage, 0)
	return nil
}

func (service *Service) executePlanning(ctx context.Context, state *runState) *stageFailure {
	stage := state.planningStage()
	if failure := state.beginStage(ctx, stage, 0); failure != nil {
		return failure
	}

	input := state.input
	input.AnswerOnly = stage == contract.WorkflowStageAnswering
	plan, err := invokeAgent(ctx, service, state, stage, func(alternate bool) (contract.Plan, error) {
		if alternate {
			return service.fallbacks.Planner.Plan(ctx, input)
		}
		return service.planner.Plan(ctx, input)
	})
	if err != nil {
		return failureAt(stage, contract.FailureCodeAgent, err, 0)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, contract.FailureCodeAgent, err, 0)
	}
	if err := plan.Validate(); err != nil {
		return failureAt(
			stage,
			contract.FailureCodeInvalidOutput,
			fmt.Errorf("invalid planner output: %w", err),
			0,
		)
	}

	if input.AnswerOnly && plan.Action != contract.PlanActionAnswer && plan.Action != contract.PlanActionPropose {
		return failureAt(stage, contract.FailureCodeInvalidOutput, errors.New("answer-only agent returned an implementation plan; no changes were started"), 0)
	}
	if input.PlanOnly && plan.Action == contract.PlanActionImplement {
		return failureAt(stage, contract.FailureCodeInvalidOutput, errors.New("plan-only request returned an implementation action; no changes were started"), 0)
	}
	if input.SelectedPlanStale && plan.Action == contract.PlanActionImplement {
		return failureAt(stage, contract.FailureCodeWorkspace, errors.New("selected plan is stale; refresh it against the current workspace before implementation"), 0)
	}
	if input.SelectedPlan != nil && plan.Action == contract.PlanActionImplement {
		selected := *input.SelectedPlan
		selected.Action = contract.PlanActionImplement
		plan = selected
	}
	if plan.Action == contract.PlanActionPropose {
		if input.SelectedPlan != nil && input.SelectedPlan.CaseID != "" {
			plan.CaseID = input.SelectedPlan.CaseID
			plan.Version = input.SelectedPlan.Version + 1
		} else {
			plan.CaseID, plan.Version = input.CaseArtifactID, 1
		}
	}
	if plan.ID == "" && state.input.PlanArtifactID != "" && (plan.Action == contract.PlanActionImplement || plan.Action == contract.PlanActionPropose) {
		plan.ID = state.input.PlanArtifactID
		if plan.Version == 0 {
			plan.Version = 1
		}
		if plan.CaseID == "" {
			plan.CaseID = state.input.CaseArtifactID
		}
	}
	state.plan = &plan
	state.events.stageCompleted(stage, 0)
	return nil
}

func (service *Service) executeDecidedPlanning(ctx context.Context, state *runState) *stageFailure {
	if state.input.PlanOnly {
		return service.executePlanning(ctx, state)
	}
	if service.decisionMaker == nil {
		return service.executePlanning(ctx, state)
	}
	if failure := state.beginStage(ctx, contract.WorkflowStageRouting, 0); failure != nil {
		return failure
	}
	if err := ctx.Err(); err != nil {
		return failureAt(contract.WorkflowStageRouting, contract.FailureCodeInternal, err, 0)
	}
	decision, err := service.decisionMaker.DecidePlanning(ctx, state.input)
	if ctx.Err() != nil {
		return failureAt(contract.WorkflowStageRouting, contract.FailureCodeInternal, ctx.Err(), 0)
	}
	if err != nil || decision.Validate() != nil {
		reason := contract.RoutingInvalid
		if err != nil {
			reason = contract.RoutingUnavailable
		}
		decision = contract.PlanningDecision{Route: contract.RoutePlan, Source: contract.DecisionFallback, Fallback: reason, NeedsPlanning: true}
	}
	state.routing = &decision
	state.events.publish(Event{Type: EventTypeRoutingDecided, Stage: contract.WorkflowStageRouting, Route: decision.Route, DecisionSource: decision.Source, RoutingFallback: decision.Fallback, Confidence: decision.Confidence})
	state.events.stageCompleted(contract.WorkflowStageRouting, 0)
	// A caller's explicit read-only constraint can never be relaxed by routing.
	if decision.Route == contract.RouteImplement && !state.input.AnswerOnly {
		if state.input.SelectedPlanStale {
			return failureAt(contract.WorkflowStageRouting, contract.FailureCodeWorkspace, errors.New("selected plan is stale; refresh it against the current workspace before implementation"), 0)
		}
		if state.input.SelectedPlan != nil {
			selected := *state.input.SelectedPlan
			selected.Action = contract.PlanActionImplement
			state.plan = &selected
			return nil
		}
		synth := syntheticPlan(state.input)
		if err := synth.Validate(); err != nil {
			return failureAt(contract.WorkflowStageRouting, contract.FailureCodeInvalidOutput, err, 0)
		}
		state.plan = &synth
		return nil
	}
	return service.executePlanning(ctx, state)
}

func syntheticPlan(input contract.TaskInput) contract.Plan {
	summary := input.Task
	if runes := []rune(summary); len(runes) > 200 {
		summary = string(runes[:200]) + "..."
	}
	return contract.Plan{
		ID:                 input.PlanArtifactID,
		CaseID:             input.CaseArtifactID,
		Version:            1,
		Action:             contract.PlanActionImplement,
		Summary:            summary,
		Steps:              []string{"Implement task as requested: " + input.Task},
		AcceptanceCriteria: []string{"Task completed per description", "No regressions introduced"},
	}
}

func (service *Service) executeDecidedReview(ctx context.Context, state *runState) *stageFailure {
	if service.decisionMaker == nil || !state.validation.Passed || len(state.validation.Checks) == 0 {
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
		if failure := state.beginStage(ctx, contract.WorkflowStageReview, state.repairAttempts); failure != nil {
			return failure
		}
		if err := state.inspect(ctx, true); err != nil {
			return failureAt(contract.WorkflowStageReview, contract.FailureCodeWorkspace, err, state.repairAttempts)
		}
		synth := contract.Review{
			Approved: decision.Approved,
			Summary:  "Auto-approved by Jev decision model: " + decision.Reason,
			Findings: nil,
		}
		if !synth.Approved {
			synth.Findings = []contract.ReviewFinding{{
				Severity:       contract.FindingSeverityWarning,
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
		state.events.stageCompleted(contract.WorkflowStageReview, state.repairAttempts)
		return nil
	}
	return service.executeReview(ctx, state)
}

func (service *Service) executeInitialImplementation(
	ctx context.Context,
	state *runState,
) *stageFailure {
	const stage = contract.WorkflowStageImplementation
	if failure := state.beginStage(ctx, stage, 0); failure != nil {
		return failure
	}
	// Only an implementation plan needs a baseline. Capture current user work
	// before any mutating agent runs; planning relies on read-only provider policy.
	if err := service.prepareWorkspace(ctx, state); err != nil {
		if errors.Is(err, errNilWorkspaceSession) {
			return failureAt(stage, contract.FailureCodeInternal, err, 0)
		}
		return failureAt(stage, contract.FailureCodeWorkspace, err, 0)
	}

	request := state.implementationRequest()
	if err := request.Validate(); err != nil {
		return failureAt(stage, contract.FailureCodeInternal, err, 0)
	}
	implementation, err := invokeAgent(ctx, service, state, stage, func(alternate bool) (contract.ImplementationResult, error) {
		if alternate {
			return service.fallbacks.Implementer.Implement(ctx, state.implementationRequest())
		}
		return service.implementer.Implement(ctx, state.implementationRequest())
	})
	inspectionErr := state.inspect(ctx, false)
	if err != nil {
		return failureAt(stage, contract.FailureCodeAgent, errors.Join(err, inspectionErr), 0)
	}
	if inspectionErr != nil {
		return failureAt(stage, contract.FailureCodeWorkspace, inspectionErr, 0)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, contract.FailureCodeAgent, err, 0)
	}
	if err := implementation.Validate(); err != nil {
		return failureAt(
			stage,
			contract.FailureCodeInvalidOutput,
			fmt.Errorf("invalid implementer output: %w", err),
			0,
		)
	}

	state.setImplementation(implementation)
	state.events.stageCompletedWithHandoff(stage, 0, state.handoffDiag())
	return nil
}

func (service *Service) executeValidation(ctx context.Context, state *runState) *stageFailure {
	const stage = contract.WorkflowStageValidation
	attempt := state.repairAttempts
	if failure := state.beginStage(ctx, stage, attempt); failure != nil {
		return failure
	}
	if err := state.inspect(ctx, true); err != nil {
		return failureAt(stage, contract.FailureCodeWorkspace, err, attempt)
	}

	request := state.validationRequest()
	if err := request.Validate(); err != nil {
		return failureAt(stage, contract.FailureCodeInternal, err, attempt)
	}
	validation, err := service.validator.Validate(ctx, request)
	// Retain completed command evidence even when a later command failed.
	validationErr := validation.Validate()
	if validationErr == nil {
		state.setValidation(validation)
	}
	inspectionErr := state.inspect(ctx, true)
	if err != nil {
		return failureAt(stage, contract.FailureCodeValidation, errors.Join(err, inspectionErr), attempt)
	}
	if inspectionErr != nil {
		return failureAt(stage, contract.FailureCodeWorkspace, inspectionErr, attempt)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, contract.FailureCodeValidation, err, attempt)
	}
	if validationErr != nil {
		return failureAt(
			stage,
			contract.FailureCodeInvalidOutput,
			fmt.Errorf("invalid validator output: %w", validationErr),
			attempt,
		)
	}

	state.events.stageCompletedWithHandoff(stage, attempt, state.handoffDiag())
	return nil
}

func (service *Service) executeReview(ctx context.Context, state *runState) *stageFailure {
	const stage = contract.WorkflowStageReview
	attempt := state.repairAttempts
	if failure := state.beginStage(ctx, stage, attempt); failure != nil {
		return failure
	}
	if err := state.inspect(ctx, true); err != nil {
		return failureAt(stage, contract.FailureCodeWorkspace, err, attempt)
	}

	request := state.reviewRequest()
	if err := request.Validate(); err != nil {
		return failureAt(stage, contract.FailureCodeInternal, err, attempt)
	}
	_, chunkBytes := service.handoffBudget()
	batches := BuildReviewBatches(request, chunkBytes)
	var review contract.Review
	if len(batches) == 1 {
		var failure *stageFailure
		review, failure = service.reviewCall(ctx, state, attempt, func(reviewer Reviewer) (contract.Review, error) {
			return reviewer.Review(ctx, state.reviewRequest())
		})
		if failure != nil {
			return failure
		}
	} else {
		_, synthesize := service.reviewer.(BatchReviewer)
		// Stop before provider execution when the chunks (plus synthesis)
		// cannot fit in the remaining invocation policy.
		needed := len(batches)
		if synthesize {
			needed++
		}
		if remaining := service.execution.MaxAgentInvocations - state.agentInvocations; needed > remaining {
			return failureAt(stage, contract.FailureCodeAgent, errors.Join(&invocationLimitError{},
				fmt.Errorf("review needs %d bounded calls but only %d invocations remain; split the task", needed, remaining)), attempt)
		}
		chunkReviews := make([]contract.Review, 0, len(batches)+1)
		for _, chunk := range batches {
			chunkReview, failure := service.reviewCall(ctx, state, attempt, func(reviewer Reviewer) (contract.Review, error) {
				if batch, ok := reviewer.(BatchReviewer); ok {
					return batch.ReviewChunk(ctx, request, chunk)
				}
				return reviewer.Review(ctx, ChunkReviewRequest(request, chunk))
			})
			if failure != nil {
				return failure
			}
			chunkReviews = append(chunkReviews, chunkReview)
		}
		review = AggregateChunkReviews(chunkReviews, *state.validation)
		if synthesize {
			// Cross-chunk judgement from collected findings, without resending diffs.
			summaries := make([]string, 0, len(chunkReviews))
			for _, chunkReview := range chunkReviews {
				summaries = append(summaries, chunkReview.Summary)
			}
			collected := review.Findings
			synthesis, failure := service.reviewCall(ctx, state, attempt, func(reviewer Reviewer) (contract.Review, error) {
				if batch, ok := reviewer.(BatchReviewer); ok {
					return batch.ReviewSynthesis(ctx, request, collected, summaries)
				}
				return review, nil
			})
			if failure != nil {
				return failure
			}
			// Chunk rejections stay authoritative; synthesis can only add findings.
			review = AggregateChunkReviews(append(chunkReviews, synthesis), *state.validation)
			review.Summary = synthesis.Summary
		}
		if err := review.Validate(); err != nil {
			return failureAt(stage, contract.FailureCodeInvalidOutput, fmt.Errorf("invalid aggregated review: %w", err), attempt)
		}
	}
	if review.Approved && !state.validation.Passed {
		return failureAt(
			stage,
			contract.FailureCodeInvalidOutput,
			errors.New("reviewer approved an implementation with failed deterministic validation"),
			attempt,
		)
	}

	state.review = &review
	if !review.Approved {
		state.events.stageProgress(stage, attempt, state.blockingFindingCount())
	}
	diag := state.handoffDiag()
	diag.ReviewChunkCount = len(batches)
	state.events.stageCompletedWithHandoff(stage, attempt, diag)
	return nil
}

// reviewCall runs one read-only reviewer invocation (primary or billing
// fallback) and requires the inspected workspace to remain unchanged.
func (service *Service) reviewCall(ctx context.Context, state *runState, attempt int, call func(Reviewer) (contract.Review, error)) (contract.Review, *stageFailure) {
	const stage = contract.WorkflowStageReview
	review, err := invokeAgent(ctx, service, state, stage, func(alternate bool) (contract.Review, error) {
		if alternate {
			return call(service.fallbacks.Reviewer)
		}
		return call(service.reviewer)
	})
	inspectionErr := state.inspect(ctx, true)
	if err != nil {
		return review, failureAt(stage, contract.FailureCodeAgent, errors.Join(err, inspectionErr), attempt)
	}
	if inspectionErr != nil {
		return review, failureAt(stage, contract.FailureCodeWorkspace, inspectionErr, attempt)
	}
	if err := ctx.Err(); err != nil {
		return review, failureAt(stage, contract.FailureCodeAgent, err, attempt)
	}
	if err := review.Validate(); err != nil {
		return review, failureAt(stage, contract.FailureCodeInvalidOutput, fmt.Errorf("invalid reviewer output: %w", err), attempt)
	}
	return review, nil
}

func (service *Service) executeRepair(ctx context.Context, state *runState) *stageFailure {
	const stage = contract.WorkflowStageRepair
	attempt := state.repairAttempts + 1
	if failure := state.beginStage(ctx, stage, attempt); failure != nil {
		return failure
	}
	if err := state.inspect(ctx, true); err != nil {
		return failureAt(stage, contract.FailureCodeWorkspace, err, attempt)
	}

	request := state.repairRequest()
	if err := request.Validate(); err != nil {
		return failureAt(stage, contract.FailureCodeInternal, err, attempt)
	}
	implementation, err := invokeAgent(ctx, service, state, stage, func(alternate bool) (contract.ImplementationResult, error) {
		state.repairAttempts = attempt
		if alternate {
			fresh := state.repairRequest()
			fresh.Implementation.AgentSessionID = "" // Sessions never cross provider boundaries.
			return service.fallbacks.Implementer.ApplyReview(ctx, fresh)
		}
		return service.implementer.ApplyReview(ctx, state.repairRequest())
	})
	inspectionErr := state.inspect(ctx, false)
	if err != nil {
		return failureAt(stage, contract.FailureCodeAgent, errors.Join(err, inspectionErr), attempt)
	}
	if inspectionErr != nil {
		return failureAt(stage, contract.FailureCodeWorkspace, inspectionErr, attempt)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, contract.FailureCodeAgent, err, attempt)
	}
	if err := implementation.Validate(); err != nil {
		return failureAt(
			stage,
			contract.FailureCodeInvalidOutput,
			fmt.Errorf("invalid repair output: %w", err),
			attempt,
		)
	}

	state.setImplementation(implementation)
	state.events.stageCompletedWithHandoff(stage, attempt, state.handoffDiag())
	return nil
}
