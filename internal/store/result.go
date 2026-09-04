package store

import (
	"fmt"
	"slices"
	"strings"
)

// WorkflowStage identifies where a terminal failure occurred.
type WorkflowStage string

const (
	WorkflowStageIntake         WorkflowStage = "intake"
	WorkflowStagePlanning       WorkflowStage = "planning"
	WorkflowStageImplementation WorkflowStage = "implementation"
	WorkflowStageValidation     WorkflowStage = "validation"
	WorkflowStageReview         WorkflowStage = "review"
	WorkflowStageRepair         WorkflowStage = "repair"
)

func (stage WorkflowStage) valid() bool {
	switch stage {
	case WorkflowStageIntake,
		WorkflowStagePlanning,
		WorkflowStageImplementation,
		WorkflowStageValidation,
		WorkflowStageReview,
		WorkflowStageRepair:
		return true
	default:
		return false
	}
}

// FailureCode classifies failures without requiring consumers to parse the
// human-readable message.
type FailureCode string

const (
	FailureCodeInvalidInput    FailureCode = "invalid_input"
	FailureCodeAgent           FailureCode = "agent_error"
	FailureCodeCommand         FailureCode = "command_error"
	FailureCodeInvalidOutput   FailureCode = "invalid_output"
	FailureCodeValidation      FailureCode = "validation_error"
	FailureCodeInternal        FailureCode = "internal_error"
	FailureCodeWorkspace       FailureCode = "workspace_error"
	FailureCodeInvocationLimit FailureCode = "invocation_limit_reached"
)

func (code FailureCode) valid() bool {
	switch code {
	case FailureCodeInvalidInput,
		FailureCodeAgent,
		FailureCodeCommand,
		FailureCodeInvalidOutput,
		FailureCodeValidation,
		FailureCodeWorkspace,
		FailureCodeInvocationLimit,
		FailureCodeInternal:
		return true
	default:
		return false
	}
}

// TaskFailure contains the structured reason for a failed workflow.
type TaskFailure struct {
	Stage    WorkflowStage    `json:"stage"`
	Code     FailureCode      `json:"code"`
	Message  string           `json:"message"`
	Provider *ProviderFailure `json:"provider,omitempty"`
}

// Validate checks a structured task failure.
func (failure TaskFailure) Validate() error {
	if failure.Provider != nil {
		if failure.Code != FailureCodeAgent {
			return invalid("provider", "provider details require agent_error")
		}
		if err := failure.Provider.Validate(); err != nil {
			return nested("provider", err)
		}
	}
	if !failure.Stage.valid() {
		return invalid("stage", fmt.Sprintf("unsupported value %q", failure.Stage))
	}
	if !failure.Code.valid() {
		return invalid("code", fmt.Sprintf("unsupported value %q", failure.Code))
	}
	if strings.TrimSpace(failure.Message) == "" {
		return invalid("message", "must not be blank")
	}
	return nil
}

// TaskStatus is a terminal workflow outcome.
type TaskStatus string

const (
	TaskStatusAnswered           TaskStatus = "answered"
	TaskStatusApproved           TaskStatus = "approved"
	TaskStatusFailed             TaskStatus = "failed"
	TaskStatusCancelled          TaskStatus = "cancelled"
	TaskStatusRepairLimitReached TaskStatus = "repair_limit_reached"
)

func (status TaskStatus) valid() bool {
	switch status {
	case TaskStatusAnswered, TaskStatusApproved,
		TaskStatusFailed,
		TaskStatusCancelled,
		TaskStatusRepairLimitReached:
		return true
	default:
		return false
	}
}

// TaskOutput is the final, machine-readable result of a workflow run.
type TaskOutput struct {
	AgentSwitches    []AgentSwitch         `json:"agent_switches,omitempty"`
	Repository       *RepositoryEvidence   `json:"repository,omitempty"`
	Status           TaskStatus            `json:"status"`
	Summary          string                `json:"summary"`
	Plan             *Plan                 `json:"plan,omitempty"`
	Implementation   *ImplementationResult `json:"implementation,omitempty"`
	Validation       *ValidationReport     `json:"validation,omitempty"`
	LastReview       *Review               `json:"last_review,omitempty"`
	RepairAttempts   int                   `json:"repair_attempts"`
	AgentInvocations int                   `json:"agent_invocations"`
	Failure          *TaskFailure          `json:"failure,omitempty"`
}

// Validate checks that a final task result contains the evidence required by
// its terminal status.
func (output TaskOutput) Validate() error {
	roles := map[WorkflowStage]bool{}
	for _, switched := range output.AgentSwitches {
		if err := switched.Validate(); err != nil {
			return nested("agent_switches", err)
		}
		role := switched.Stage
		if role == WorkflowStageRepair {
			role = WorkflowStageImplementation
		}
		if roles[role] {
			return invalid("agent_switches", "a role may switch at most once per run")
		}
		roles[role] = true
	}
	if !output.Status.valid() {
		return invalid("status", fmt.Sprintf("unsupported value %q", output.Status))
	}
	if strings.TrimSpace(output.Summary) == "" {
		return invalid("summary", "must not be blank")
	}
	if output.AgentInvocations < 0 {
		return invalid("agent_invocations", "must be nonnegative")
	}
	if output.RepairAttempts < 0 {
		return invalid("repair_attempts", "must be zero or greater")
	}
	if err := validateRepository(output.Repository); err != nil {
		return err
	}
	if output.Status != TaskStatusFailed && output.Failure != nil {
		return invalid("failure", "is only valid for failed status")
	}

	switch output.Status {
	case TaskStatusAnswered:
		if output.Plan == nil || output.Plan.Action != PlanActionAnswer {
			return invalid("plan", "an answer-only plan is required")
		}
		if err := output.Plan.Validate(); err != nil {
			return nested("plan", err)
		}
		if output.Summary != output.Plan.Answer {
			return invalid("summary", "must contain the planner's answer")
		}
		if output.Implementation != nil || output.Validation != nil || output.LastReview != nil || output.RepairAttempts != 0 {
			return invalid("plan", "an answer cannot contain implementation, validation, review, or repair evidence")
		}
		if output.Repository == nil || !output.Repository.Complete || len(output.Repository.PreservationViolations) != 0 || len(output.Repository.ChangedFiles) != 0 || output.Repository.Baseline != output.Repository.Current {
			return invalid("repository", "answer-only completion requires an unchanged, completely inspected workspace")
		}
	case TaskStatusApproved:
		if err := output.validateCompletedEvidence(); err != nil {
			return err
		}
		if !output.Validation.Passed {
			return invalid("validation.passed", "must be true for approved status")
		}
		if !output.LastReview.Approved {
			return invalid("last_review.approved", "must be true for approved status")
		}
	case TaskStatusFailed:
		if output.Failure == nil {
			return invalid("failure", "is required for failed status")
		}
		if err := output.Failure.Validate(); err != nil {
			return nested("failure", err)
		}
	case TaskStatusRepairLimitReached:
		if err := output.validateCompletedEvidence(); err != nil {
			return err
		}
		if output.LastReview.Approved {
			return invalid("last_review.approved", "must be false when the repair limit is reached")
		}
	case TaskStatusCancelled:
		// A cancellation can occur before any plan or implementation exists.
	}
	return nil
}

func (output TaskOutput) validateCompletedEvidence() error {
	if output.Repository == nil || !output.Repository.Complete {
		return invalid("repository", "complete independent repository evidence is required")
	}
	if len(output.Repository.PreservationViolations) != 0 {
		return invalid("repository.preservation_violations", "cannot complete after changing protected work")
	}
	if output.Plan == nil {
		return invalid("plan", "is required for this status")
	}
	if err := output.Plan.ValidateImplementation(); err != nil {
		return nested("plan", err)
	}
	if output.Implementation == nil {
		return invalid("implementation", "is required for this status")
	}
	if err := output.Implementation.Validate(); err != nil {
		return nested("implementation", err)
	}
	if !slices.Equal(output.Implementation.ChangedFiles, output.Repository.ChangedFiles) {
		return invalid("implementation.changed_files", "must agree with independent repository evidence")
	}
	if output.Validation == nil {
		return invalid("validation", "is required for this status")
	}
	if err := output.Validation.Validate(); err != nil {
		return nested("validation", err)
	}
	if output.LastReview == nil {
		return invalid("last_review", "is required for this status")
	}
	if err := output.LastReview.Validate(); err != nil {
		return nested("last_review", err)
	}
	return nil
}
