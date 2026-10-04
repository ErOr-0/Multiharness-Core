package workflow

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/contract"
)

func (state *runState) terminalFrom(ctx context.Context, failure *stageFailure) contract.TaskOutput {
	if isCancellation(ctx) {
		return state.cancelled(failure.stage, failure.cause, failure.repairAttempt)
	}
	return state.failed(failure.stage, failure.code, failure.cause, failure.repairAttempt)
}

func (state *runState) failed(
	stage contract.WorkflowStage,
	code contract.FailureCode,
	err error,
	repairAttempt int,
) contract.TaskOutput {
	output := state.baseOutput()
	output.Status = contract.TaskStatusFailed
	output.Summary = fmt.Sprintf("workflow failed during %s", stage)
	output.Failure = &contract.TaskFailure{Stage: stage, Code: code, Message: err.Error()}
	if code == contract.FailureCodeValidationInput {
		output.Status = contract.TaskStatusNeedsInput
		output.Summary = "workflow needs validation authorization or configuration"
	}
	var limit *invocationLimitError
	var provider *contract.ProviderFailure
	if errors.As(err, &limit) {
		output.Failure.Code = contract.FailureCodeInvocationLimit
	}
	if errors.As(err, &provider) && provider != nil && code == contract.FailureCodeAgent && provider.Validate() == nil {
		details := *provider
		output.Failure.Provider = &details
	}
	if denied := permissionOnly(err); denied != nil && code == contract.FailureCodeAgent && denied.Validate() == nil {
		details := *denied
		output.Status = contract.TaskStatusNeedsInput
		output.Summary = fmt.Sprintf("workflow needs permission during %s", stage)
		output.Failure.Code = contract.FailureCodePermission
		output.Failure.Permission = &details
	}
	state.events.stageFailed(stage, output.Status, output.Failure.Code, repairAttempt)
	state.events.workflowCompleted(stage, output.Status)
	return output
}

// A simultaneous workspace/process failure must not be hidden as a permission
// request. Unwrap adapter context, but require every joined cause to be a denial.
func permissionOnly(err error) *contract.PermissionDenied {
	if denied, ok := err.(*contract.PermissionDenied); ok {
		return denied
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var found *contract.PermissionDenied
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
	stage contract.WorkflowStage,
	err error,
	repairAttempt int,
) contract.TaskOutput {
	output := state.baseOutput()
	output.Status = contract.TaskStatusCancelled
	output.Summary = fmt.Sprintf("workflow cancelled during %s: %v", stage, err)
	state.events.stageFailed(stage, output.Status, "", repairAttempt)
	state.events.workflowCompleted(stage, output.Status)
	return output
}

func (state *runState) answered() contract.TaskOutput {
	output := state.baseOutput()
	output.Status = contract.TaskStatusAnswered
	output.Summary = state.plan.Display()
	if err := output.Validate(); err != nil {
		return state.failed(state.planningStage(), contract.FailureCodeInternal, err, 0)
	}
	state.events.workflowCompleted(state.planningStage(), output.Status)
	return output
}

func (state *runState) approved() contract.TaskOutput {
	output := state.baseOutput()
	output.Status = contract.TaskStatusApproved
	output.Summary = state.review.Summary
	if err := output.Validate(); err != nil {
		return state.failed(contract.WorkflowStageReview, contract.FailureCodeInternal, err, state.repairAttempts)
	}
	state.events.workflowCompleted(contract.WorkflowStageReview, output.Status)
	return output
}

func (state *runState) repairLimitReached() contract.TaskOutput {
	output := state.baseOutput()
	output.Status = contract.TaskStatusRepairLimitReached
	output.Summary = state.review.Summary
	if err := output.Validate(); err != nil {
		return state.failed(contract.WorkflowStageReview, contract.FailureCodeInternal, err, state.repairAttempts)
	}
	state.events.workflowCompleted(contract.WorkflowStageReview, output.Status)
	return output
}

func (state *runState) baseOutput() contract.TaskOutput {
	return normalizeTaskOutput(contract.TaskOutput{
		Routing:          state.routing,
		Repository:       state.repository,
		Plan:             state.plan,
		Implementation:   state.implementation,
		Validation:       state.validation,
		LastReview:       state.review,
		RepairAttempts:   state.repairAttempts,
		AgentInvocations: state.agentInvocations,
		AgentSwitches:    append([]contract.AgentSwitch(nil), state.agentSwitches...),
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
	stage         contract.WorkflowStage
	code          contract.FailureCode
	cause         error
	repairAttempt int
}

func failureAt(
	stage contract.WorkflowStage,
	code contract.FailureCode,
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
func normalizeTaskOutput(output contract.TaskOutput) contract.TaskOutput {
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
			validation.Checks = []contract.ValidationEvidence{}
		}
		output.Validation = &validation
	}
	if output.LastReview != nil {
		review := *output.LastReview
		if review.Findings == nil {
			review.Findings = []contract.ReviewFinding{}
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
