package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"multiharness-core/internal/contract"
)

type ValidationApprover interface {
	ConfirmValidation(context.Context, string, contract.ValidationAction) (bool, error)
}

type RequestedValidator interface {
	ValidateAction(context.Context, contract.ValidationRequest, contract.ValidationAction) (contract.ValidationReport, error)
}

func (s *Service) executeRequestedValidation(ctx context.Context, state *runState) *stageFailure {
	stage := contract.WorkflowStageValidation
	fail := func(code contract.FailureCode, err error) *stageFailure {
		return failureAt(stage, code, err, state.repairAttempts)
	}
	if err := ctx.Err(); err != nil {
		return fail(contract.FailureCodeValidation, err)
	}
	action := *state.review.ValidationAction
	action.Args = append([]string{}, action.Args...)
	if err := action.Validate(); err != nil {
		return fail(contract.FailureCodeInvalidOutput, err)
	}
	keyBytes, _ := json.Marshal(struct {
		Executable string
		Args       []string
		Revision   string
	}{action.Executable, action.Args, state.repository.Current.Fingerprint})
	key := string(keyBytes)
	if state.validationActionCount >= 4 || state.validationActions[key] {
		return fail(contract.FailureCodeValidationInput, errors.New("validation request repeated without a workspace change or request limit reached; inspect the validation evidence and resolve the environment before retrying; no repair attempt was consumed"))
	}
	validator, ok := s.validator.(RequestedValidator)
	if !ok || s.validationApprover == nil {
		return fail(contract.FailureCodeValidationInput, errors.New("review requested validation; an interactive terminal is required to authorize the command, or configure validation.checks before retrying"))
	}
	if err := state.inspect(ctx, true); err != nil {
		return fail(contract.FailureCodeWorkspace, err)
	}
	yes, err := s.validationApprover.ConfirmValidation(ctx, state.input.WorkingDir, action)
	if err != nil {
		return fail(contract.FailureCodeValidation, err)
	}
	if err := ctx.Err(); err != nil {
		return fail(contract.FailureCodeValidation, err)
	}
	if !yes {
		return fail(contract.FailureCodeValidationInput, errors.New("validation command declined; no command was run and no repair attempt was consumed; edits were kept"))
	}
	// Detect changes while the user was reading the prompt before running anything.
	if err := state.inspect(ctx, true); err != nil {
		return fail(contract.FailureCodeWorkspace, err)
	}
	if state.validationActions == nil {
		state.validationActions = map[string]bool{}
	}
	state.validationActions[key] = true
	state.validationActionCount++
	state.events.publish(Event{Type: EventTypeStageStarted, Stage: stage, RepairAttempt: state.repairAttempts, AuthorizedValidation: true})
	if err := ctx.Err(); err != nil {
		return fail(contract.FailureCodeValidation, err)
	}
	report, runErr := validator.ValidateAction(ctx, state.validationRequest(), action)
	// Explicitly authorized commands may create build artifacts or update dependencies.
	// Record the resulting diff, while retaining protections for pre-existing work.
	inspectErr := state.inspect(ctx, false)
	if report.Validate() == nil {
		state.validation = &report
	}
	if inspectErr != nil {
		return fail(contract.FailureCodeWorkspace, errors.Join(runErr, inspectErr))
	}
	if runErr != nil {
		return fail(contract.FailureCodeValidation, runErr)
	}
	if err := report.Validate(); err != nil {
		return fail(contract.FailureCodeInvalidOutput, fmt.Errorf("requested validation: %w", err))
	}
	if len(report.Checks) == 0 {
		return fail(contract.FailureCodeInvalidOutput, errors.New("requested validation returned no command evidence"))
	}
	// Re-run configured checks against the new state; stale earlier failures must not
	// override fresh evidence, and approving one command must not skip configured gates.
	configured, checkErr := s.validator.Validate(ctx, state.validationRequest())
	inspectErr = state.inspect(ctx, true)
	if inspectErr != nil {
		return fail(contract.FailureCodeWorkspace, errors.Join(checkErr, inspectErr))
	}
	if err := configured.Validate(); err != nil {
		return fail(contract.FailureCodeInvalidOutput, errors.Join(checkErr, err))
	}
	configured.Checks = append(configured.Checks, report.Checks...)
	configured.Passed = configured.Passed && report.Passed
	state.setValidation(configured)
	if checkErr != nil {
		return fail(contract.FailureCodeValidation, checkErr)
	}
	if err := ctx.Err(); err != nil {
		return fail(contract.FailureCodeValidation, err)
	}
	state.events.stageCompleted(stage, state.repairAttempts)
	return nil
}
