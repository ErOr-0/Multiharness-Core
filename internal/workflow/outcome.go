package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/store"
)

func (state *runState) terminalFrom(ctx context.Context, failure *stageFailure) store.TaskOutput {
	if isCancellation(ctx) {
		return state.cancelled(failure.stage, failure.cause, failure.repairAttempt)
	}
	return state.failed(failure.stage, failure.code, failure.cause, failure.repairAttempt)
}

func (state *runState) failed(
	stage store.WorkflowStage,
	code store.FailureCode,
	err error,
	repairAttempt int,
) store.TaskOutput {
	output := state.baseOutput()
	output.Status = store.TaskStatusFailed
	output.Summary = fmt.Sprintf("workflow failed during %s", stage)
	output.Failure = &store.TaskFailure{Stage: stage, Code: code, Message: err.Error()}
	var limit *invocationLimitError
	var provider *store.ProviderFailure
	if errors.As(err, &limit) {
		output.Failure.Code = store.FailureCodeInvocationLimit
	}
	if errors.As(err, &provider) && provider != nil && code == store.FailureCodeAgent && provider.Validate() == nil {
		details := *provider
		output.Failure.Provider = &details
	}
	if denied := permissionOnly(err); denied != nil && code == store.FailureCodeAgent && denied.Validate() == nil {
		details := *denied
		output.Status = store.TaskStatusNeedsInput
		output.Summary = fmt.Sprintf("workflow needs permission during %s", stage)
		output.Failure.Code = store.FailureCodePermission
		output.Failure.Permission = &details
	}
	state.events.stageFailed(stage, output.Status, output.Failure.Code, repairAttempt)
	state.events.workflowCompleted(stage, output.Status)
	return output
}

// A simultaneous workspace/process failure must not be hidden as a permission
// request. Unwrap adapter context, but require every joined cause to be a denial.
func permissionOnly(err error) *store.PermissionDenied {
	if denied, ok := err.(*store.PermissionDenied); ok {
		return denied
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var found *store.PermissionDenied
		for _, cause := range joined.Unwrap() {
			denied := permissionOnly(cause)
			if denied == nil {
				return nil
			}
			found = denied
		}
		return found
	}
	if cause := errors.Unwrap(err); cause != nil {
		return permissionOnly(cause)
	}
	return nil
}

func (state *runState) cancelled(
	stage store.WorkflowStage,
	err error,
	repairAttempt int,
) store.TaskOutput {
	output := state.baseOutput()
	output.Status = store.TaskStatusCancelled
	output.Summary = fmt.Sprintf("workflow cancelled during %s: %v", stage, err)
	state.events.stageFailed(stage, output.Status, "", repairAttempt)
	state.events.workflowCompleted(stage, output.Status)
	return output
}

func (state *runState) answered() store.TaskOutput {
	output := state.baseOutput()
	output.Status = store.TaskStatusAnswered
	output.Summary = state.plan.Display()
	if err := output.Validate(); err != nil {
		return state.failed(state.planningStage(), store.FailureCodeInternal, err, 0)
	}
	state.events.workflowCompleted(state.planningStage(), output.Status)
	return output
}

func (state *runState) approved() store.TaskOutput {
	output := state.baseOutput()
	output.Status = store.TaskStatusApproved
	output.Summary = state.review.Summary
	if err := output.Validate(); err != nil {
		return state.failed(store.WorkflowStageReview, store.FailureCodeInternal, err, state.repairAttempts)
	}
	state.events.workflowCompleted(store.WorkflowStageReview, output.Status)
	return output
}

func (state *runState) repairLimitReached() store.TaskOutput {
	output := state.baseOutput()
	output.Status = store.TaskStatusRepairLimitReached
	output.Summary = state.review.Summary
	if err := output.Validate(); err != nil {
		return state.failed(store.WorkflowStageReview, store.FailureCodeInternal, err, state.repairAttempts)
	}
	state.events.workflowCompleted(store.WorkflowStageReview, output.Status)
	return output
}

func (state *runState) baseOutput() store.TaskOutput {
	return normalizeTaskOutput(store.TaskOutput{
		Routing:          state.routing,
		Repository:       state.repository,
		Plan:             state.plan,
		Implementation:   state.implementation,
		Validation:       state.validation,
		LastReview:       state.review,
		RepairAttempts:   state.repairAttempts,
		AgentInvocations: state.agentInvocations,
		AgentSwitches:    append([]store.AgentSwitch(nil), state.agentSwitches...),
	})
}

func isCancellation(ctx context.Context) bool {
	// An agent can stop its own invocation while the caller is still active.
	// Only cancellation of the workflow context cancels the whole task.
	return ctx != nil && ctx.Err() != nil
}

// stageFailure carries failure context from one stage executor to the
// orchestration boundary. It is converted there into a terminal task output.
type stageFailure struct {
	stage         store.WorkflowStage
	code          store.FailureCode
	cause         error
	repairAttempt int
}

func failureAt(
	stage store.WorkflowStage,
	code store.FailureCode,
	cause error,
	repairAttempt int,
) *stageFailure {
	return &stageFailure{
		stage:         stage,
		code:          code,
		cause:         cause,
		repairAttempt: repairAttempt,
	}
}

// normalizeTaskOutput preserves domain meaning while keeping empty collections
// in returned evidence serializable as JSON arrays rather than null. Optional
// evidence pointers stay nil when their stage has not produced a result.
func normalizeTaskOutput(output store.TaskOutput) store.TaskOutput {
	if output.Repository != nil {
		repository := output.Repository.Clone()
		repository.ChangedFiles = stringsOrEmpty(repository.ChangedFiles)
		repository.PreExistingFiles = stringsOrEmpty(repository.PreExistingFiles)
		repository.PreservationViolations = stringsOrEmpty(repository.PreservationViolations)
		output.Repository = repository
	}
	if output.Plan != nil {
		plan := *output.Plan
		plan.Steps = stringsOrEmpty(plan.Steps)
		plan.AcceptanceCriteria = stringsOrEmpty(plan.AcceptanceCriteria)
		output.Plan = &plan
	}
	if output.Implementation != nil {
		implementation := *output.Implementation
		implementation.ChangedFiles = stringsOrEmpty(implementation.ChangedFiles)
		output.Implementation = &implementation
	}
	if output.Validation != nil {
		validation := *output.Validation
		if validation.Checks == nil {
			validation.Checks = []store.ValidationEvidence{}
		}
		output.Validation = &validation
	}
	if output.LastReview != nil {
		review := *output.LastReview
		if review.Findings == nil {
			review.Findings = []store.ReviewFinding{}
		}
		review.Suggestions = stringsOrEmpty(review.Suggestions)
		output.LastReview = &review
	}
	return output
}

func stringsOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
