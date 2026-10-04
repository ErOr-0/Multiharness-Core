package workflow

import (
	"context"
	"errors"

	"multiharness-core/internal/contract"
)

type runState struct {
	validationActions     map[string]bool
	validationActionCount int
	routing               *contract.PlanningDecision
	workspace             WorkspaceSession
	repository            *contract.RepositoryEvidence
	input                 contract.TaskInput
	plan                  *contract.Plan
	implementation        *contract.ImplementationResult
	validation            *contract.ValidationReport
	review                *contract.Review
	repairAttempts        int
	agentInvocations      int
	alternateRoles        map[contract.WorkflowStage]bool
	agentSwitches         []contract.AgentSwitch
	events                *eventEmitter
}

func newRunState(input contract.TaskInput, sink EventSink) *runState {
	return &runState{input: input, events: newEventEmitter(sink)}
}

// beginStage guards every handoff, including cancellation by an event sink
// after the preceding stage completed. Stages remain ordinary function calls.
func (state *runState) beginStage(ctx context.Context, stage contract.WorkflowStage, attempt int) *stageFailure {
	if ctx == nil {
		return failureAt(stage, contract.FailureCodeInternal, errNilContext, state.repairAttempts)
	}
	if err := ctx.Err(); err != nil {
		return failureAt(stage, contract.FailureCodeInternal, err, state.repairAttempts)
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
	stage := contract.WorkflowStageReview
	if state.plan.Action == contract.PlanActionAnswer || state.plan.Action == contract.PlanActionPropose {
		stage = state.planningStage()
	}
	return failureAt(stage, contract.FailureCodeWorkspace, err, state.repairAttempts)
}

func (state *runState) setImplementation(implementation contract.ImplementationResult) {
	if implementation.ID == "" && state.input.ImplementationArtifactID != "" {
		implementation.ID, implementation.Version = state.input.ImplementationArtifactID, 1
	}
	implementation.ChangedFiles = append([]string{}, state.repository.ChangedFiles...)
	state.implementation = &implementation
	state.validation = nil
	state.review = nil
}

func (state *runState) setValidation(validation contract.ValidationReport) {
	state.validation = &validation
	state.review = nil
}

func (state *runState) implementationRequest() contract.ImplementationRequest {
	return contract.ImplementationRequest{Input: state.stageInput(), Plan: *state.plan, Repository: state.repository.Clone()}
}

func (state *runState) stageInput() contract.TaskInput {
	input := state.input
	// Native sessions belong to one provider and role. Team handoffs carry
	// explicit context, not the previous role's opaque conversation identifier.
	input.SessionID = ""
	// The selected plan is already supplied as request.plan. Avoid paying for it twice.
	input.SelectedPlan = nil
	input.SelectedPlanStale = false
	input.PlanArtifactID, input.CaseArtifactID, input.ImplementationArtifactID = "", "", ""
	return input
}

func (state *runState) validationRequest() contract.ValidationRequest {
	return contract.ValidationRequest{
		Repository:     state.repository.Clone(),
		Input:          state.stageInput(),
		Plan:           *state.plan,
		Implementation: *state.implementation,
	}
}

func (state *runState) reviewRequest() contract.ReviewRequest {
	return contract.ReviewRequest{
		Repository:     state.repository.Clone(),
		Input:          state.stageInput(),
		Plan:           *state.plan,
		Implementation: *state.implementation,
		Validation:     *state.validation,
	}
}

func (state *runState) repairRequest() contract.RepairRequest {
	return contract.RepairRequest{
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

func (state *runState) planningStage() contract.WorkflowStage {
	if state.input.AnswerOnly || (state.routing != nil && state.routing.Route == contract.RouteAnswer) {
		return contract.WorkflowStageAnswering
	}
	return contract.WorkflowStagePlanning
}
