package store

import (
	"fmt"
	"slices"
	"strings"
)

// WorkflowStage identifies where a terminal failure occurred.
type WorkflowStage string

const (
	WorkflowStageDelegation     WorkflowStage = "delegation"
	WorkflowStageIntake         WorkflowStage = "intake"
	WorkflowStagePlanning       WorkflowStage = "planning"
	WorkflowStageAnswering      WorkflowStage = "answering"
	WorkflowStageRouting        WorkflowStage = "routing"
	WorkflowStageImplementation WorkflowStage = "implementation"
	WorkflowStageValidation     WorkflowStage = "validation"
	WorkflowStageReview         WorkflowStage = "review"
	WorkflowStageRepair         WorkflowStage = "repair"
)

func (stage WorkflowStage) valid() bool {
	switch stage {
	case WorkflowStageDelegation, WorkflowStageIntake, WorkflowStageRouting, WorkflowStageAnswering,
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
	FailureCodePermission      FailureCode = "permission_denied"
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
		FailureCodePermission,
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
	Permission *PermissionDenied `json:"permission,omitempty"`
	Stage      WorkflowStage     `json:"stage"`
	Code       FailureCode       `json:"code"`
	Message    string            `json:"message"`
	Provider   *ProviderFailure  `json:"provider,omitempty"`
}

// Validate checks a structured task failure.
func (failure TaskFailure) Validate() error {
	if failure.Code == FailureCodePermission {
		if failure.Permission == nil {
			return invalid("permission", "denial evidence is required")
		}
		if err := failure.Permission.Validate(); err != nil {
			return nested("permission", err)
		}
		switch failure.Stage {
		case WorkflowStagePlanning, WorkflowStageImplementation, WorkflowStageReview, WorkflowStageRepair:
		default:
			return invalid("stage", "permission denial requires an agent stage")
		}
	} else if failure.Permission != nil {
		return invalid("permission", "requires permission_denied")
	}
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
	TaskStatusResponded          TaskStatus = "responded"
	TaskStatusNeedsInput         TaskStatus = "needs_input"
	TaskStatusTimedOut           TaskStatus = "timed_out"
	TaskStatusAnswered           TaskStatus = "answered"
	TaskStatusApproved           TaskStatus = "approved"
	TaskStatusFailed             TaskStatus = "failed"
	TaskStatusCancelled          TaskStatus = "cancelled"
	TaskStatusRepairLimitReached TaskStatus = "repair_limit_reached"
)

func (status TaskStatus) valid() bool {
	switch status {
	case TaskStatusResponded, TaskStatusNeedsInput, TaskStatusTimedOut, TaskStatusAnswered, TaskStatusApproved,
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
	RetrievedContextBytes int                   `json:"retrieved_context_bytes,omitempty"`
	Routing               *PlanningDecision     `json:"routing,omitempty"`
	Direct                *DirectResponse       `json:"direct,omitempty"`
	AgentSwitches         []AgentSwitch         `json:"agent_switches,omitempty"`
	Repository            *RepositoryEvidence   `json:"repository,omitempty"`
	Status                TaskStatus            `json:"status"`
	Summary               string                `json:"summary"`
	Plan                  *Plan                 `json:"plan,omitempty"`
	Implementation        *ImplementationResult `json:"implementation,omitempty"`
	Validation            *ValidationReport     `json:"validation,omitempty"`
	LastReview            *Review               `json:"last_review,omitempty"`
	RepairAttempts        int                   `json:"repair_attempts"`
	AgentInvocations      int                   `json:"agent_invocations"`
	Failure               *TaskFailure          `json:"failure,omitempty"`
}

// Validate checks that a final task result contains the evidence required by
// its terminal status.
func (output TaskOutput) Validate() error {
	if output.Routing != nil {
		if err := output.Routing.Validate(); err != nil {
			return err
		}
		if output.Routing.Route == RouteAnswer && (output.Implementation != nil || output.Repository != nil || output.Validation != nil || output.LastReview != nil) {
			return invalid("routing", "answer route cannot contain coding evidence")
		}
	}
	if output.Direct != nil {
		if err := output.Direct.Validate(); err != nil {
			return err
		}
		if output.Routing != nil || output.Plan != nil || output.Implementation != nil || output.Validation != nil || output.LastReview != nil || output.Repository != nil || len(output.AgentSwitches) != 0 || output.RepairAttempts != 0 {
			return invalid("direct", "cannot carry team workflow evidence")
		}
		if output.Status != TaskStatusResponded && output.Status != TaskStatusNeedsInput && output.Status != TaskStatusTimedOut && output.Status != TaskStatusCancelled && output.Status != TaskStatusFailed {
			return invalid("direct", "requires a direct outcome")
		}
	}
	roles := map[WorkflowStage]bool{}
	for _, switched := range output.AgentSwitches {
		if err := switched.Validate(); err != nil {
			return nested("agent_switches", err)
		}
		role := switched.Stage
		if role == WorkflowStageAnswering {
			role = WorkflowStagePlanning
		}
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
	if output.RetrievedContextBytes < 0 {
		return invalid("retrieved_context_bytes", "must be nonnegative")
	}
	if output.RepairAttempts < 0 {
		return invalid("repair_attempts", "must be zero or greater")
	}
	if err := validateRepository(output.Repository); err != nil {
		return err
	}
	if output.Status != TaskStatusFailed && output.Status != TaskStatusNeedsInput && output.Failure != nil {
		return invalid("failure", "is only valid for failed status")
	}

	switch output.Status {
	case TaskStatusResponded:
		if output.Direct == nil || strings.TrimSpace(output.Direct.Text) == "" || output.Direct.NeedsInput || output.Summary != output.Direct.Text {
			return invalid("direct", "requires the native final response")
		}
	case TaskStatusNeedsInput:
		if output.Direct == nil {
			if output.Failure == nil || output.Failure.Code != FailureCodePermission {
				return invalid("failure", "requires a native permission block")
			}
			if err := output.Failure.Validate(); err != nil {
				return nested("failure", err)
			}
		} else if !output.Direct.NeedsInput || output.Failure != nil {
			return invalid("direct", "requires a native input or permission block")
		}
	case TaskStatusTimedOut:
		if output.Direct == nil {
			return invalid("direct", "requires a direct invocation")
		}
	case TaskStatusAnswered:
		if output.Plan == nil || (output.Plan.Action != PlanActionAnswer && output.Plan.Action != PlanActionPropose) {
			return invalid("plan", "an answer or saved proposal is required")
		}
		if err := output.Plan.Validate(); err != nil {
			return nested("plan", err)
		}
		if output.Summary != output.Plan.Display() {
			return invalid("summary", "must contain the planner's answer")
		}
		if output.Implementation != nil || output.Validation != nil || output.LastReview != nil || output.RepairAttempts != 0 {
			return invalid("plan", "an answer cannot contain implementation, validation, review, or repair evidence")
		}
		if output.Repository != nil && (!output.Repository.Complete || len(output.Repository.PreservationViolations) != 0 || len(output.Repository.ChangedFiles) != 0 || output.Repository.Baseline != output.Repository.Current) {
			return invalid("repository", "optional answer-only repository evidence must be complete and unchanged")
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
