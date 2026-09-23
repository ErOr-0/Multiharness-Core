package workflow

import (
	"context"
	"errors"

	"multiharness-core/internal/store"
)

type runState struct {
	routing          *store.PlanningDecision
	workspace        WorkspaceSession
	repository       *store.RepositoryEvidence
	input            store.TaskInput
	plan             *store.Plan
	implementation   *store.ImplementationResult
	validation       *store.ValidationReport
	review           *store.Review
	repairAttempts   int
	agentInvocations int
	alternateRoles   map[store.WorkflowStage]bool
	agentSwitches    []store.AgentSwitch
	events           *eventEmitter
}

func newRunState(input store.TaskInput, sink EventSink) *runState {
	return &runState{input: input, events: newEventEmitter(sink)}
}

// beginStage guards every handoff, including cancellation by an event sink
// after the preceding stage completed. Stages remain ordinary function calls.
func (state *runState) beginStage(ctx context.Context, stage store.WorkflowStage, attempt int) *stageFailure {
	if ctx == nil {
		return failureAt(stage, store.FailureCodeInternal, errNilContext, state.repairAttempts)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, store.FailureCodeInternal, err, state.repairAttempts)
	}
	state.events.stageStarted(stage, attempt)
	return nil
}

func (state *runState) releaseWorkspace(failure *stageFailure) *stageFailure {
	if state.workspace == nil {
		return failure
	}
	err := state.workspace.Close()
	if err == nil {
		state.workspace = nil
		return failure
	}
	if failure != nil {
		failure.cause = errors.Join(failure.cause, err)
		return failure
	}
	stage := store.WorkflowStageReview
	if state.plan.Action == store.PlanActionAnswer || state.plan.Action == store.PlanActionPropose {
		stage = state.planningStage()
	}
	return failureAt(stage, store.FailureCodeWorkspace, err, state.repairAttempts)
}

func (state *runState) setImplementation(implementation store.ImplementationResult) {
	if implementation.ID == "" && state.input.ImplementationArtifactID != "" {
		implementation.ID, implementation.Version = state.input.ImplementationArtifactID, 1
	}
	implementation.ChangedFiles = append([]string{}, state.repository.ChangedFiles...)
	state.implementation = &implementation
	state.validation = nil
	state.review = nil
}

func (state *runState) setValidation(validation store.ValidationReport) {
	state.validation = &validation
	state.review = nil
}

func (state *runState) implementationRequest() store.ImplementationRequest {
	return store.ImplementationRequest{Input: state.stageInput(), Plan: *state.plan, Repository: state.repository.Clone()}
}

func (state *runState) stageInput() store.TaskInput {
	input := state.input
	// The selected plan is already supplied as request.plan. Avoid paying for it twice.
	input.SelectedPlan = nil
	input.SelectedPlanStale = false
	input.PlanArtifactID, input.CaseArtifactID, input.ImplementationArtifactID = "", "", ""
	return input
}

func (state *runState) validationRequest() store.ValidationRequest {
	return store.ValidationRequest{
		Repository:     state.repository.Clone(),
		Input:          state.stageInput(),
		Plan:           *state.plan,
		Implementation: *state.implementation,
	}
}

func (state *runState) reviewRequest() store.ReviewRequest {
	return store.ReviewRequest{
		Repository:     state.repository.Clone(),
		Input:          state.stageInput(),
		Plan:           *state.plan,
		Implementation: *state.implementation,
		Validation:     *state.validation,
	}
}

func (state *runState) repairRequest() store.RepairRequest {
	return store.RepairRequest{
		Repository:     state.repository.Clone(),
		Input:          state.stageInput(),
		Plan:           *state.plan,
		Implementation: *state.implementation,
		Validation:     *state.validation,
		Review:         *state.review,
	}
}

func (state *runState) blockingFindingCount() int {
	if state.review == nil {
		return 0
	}
	count := 0
	for _, finding := range state.review.Findings {
		if finding.Blocking {
			count++
		}
	}
	return count
}

func (state *runState) planningStage() store.WorkflowStage {
	if state.input.AnswerOnly || (state.routing != nil && state.routing.Route == store.RouteAnswer) {
		return store.WorkflowStageAnswering
	}
	return store.WorkflowStagePlanning
}
