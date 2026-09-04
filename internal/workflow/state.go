package workflow

import (
	"context"

	"multiharness-core/internal/store"
)

// WorkflowState represents a discrete state in the lifecycle of a task execution.
// Each state encapsulates its stage execution logic, invariant checks, and transition
// decisions to the next state in the state machine.
type WorkflowState interface {
	Stage() store.WorkflowStage
	Execute(ctx context.Context, sm *StateMachine) (WorkflowState, *stageFailure)
}

// IntakeState handles initial input validation, workspace access verification,
// and baseline repository evidence capture.
type IntakeState struct{}

func (s *IntakeState) Stage() store.WorkflowStage {
	return store.WorkflowStageIntake
}

func (s *IntakeState) Execute(ctx context.Context, sm *StateMachine) (WorkflowState, *stageFailure) {
	if failure := sm.service.executeIntake(ctx, sm.state); failure != nil {
		return nil, failure
	}
	return &PlanningState{}, nil
}

// PlanningState invokes the planning strategy to produce either a structured
// implementation plan or an answer-only response for non-coding requests.
type PlanningState struct{}

func (s *PlanningState) Stage() store.WorkflowStage {
	return store.WorkflowStagePlanning
}

func (s *PlanningState) Execute(ctx context.Context, sm *StateMachine) (WorkflowState, *stageFailure) {
	if failure := sm.service.executePlanning(ctx, sm.state); failure != nil {
		return nil, failure
	}
	// Non-coding questions terminate here with Answered terminal status.
	if sm.state.plan.Action == store.PlanActionAnswer {
		return nil, nil
	}
	return &InitialImplementationState{}, nil
}

// InitialImplementationState runs the first implementation attempt against the
// approved plan and inspects changed files.
type InitialImplementationState struct{}

func (s *InitialImplementationState) Stage() store.WorkflowStage {
	return store.WorkflowStageImplementation
}

func (s *InitialImplementationState) Execute(ctx context.Context, sm *StateMachine) (WorkflowState, *stageFailure) {
	if failure := sm.service.executeInitialImplementation(ctx, sm.state); failure != nil {
		return nil, failure
	}
	return &ValidationState{}, nil
}

// ValidationState executes deterministic validation commands and records the
// validation report before review.
type ValidationState struct{}

func (s *ValidationState) Stage() store.WorkflowStage {
	return store.WorkflowStageValidation
}

func (s *ValidationState) Execute(ctx context.Context, sm *StateMachine) (WorkflowState, *stageFailure) {
	if failure := sm.service.executeValidation(ctx, sm.state); failure != nil {
		return nil, failure
	}
	return &ReviewState{}, nil
}

// ReviewState invokes the reviewer agent with implementation evidence and
// deterministic validation results, managing the transition to approval, repair,
// or limit exhaustion.
type ReviewState struct{}

func (s *ReviewState) Stage() store.WorkflowStage {
	return store.WorkflowStageReview
}

func (s *ReviewState) Execute(ctx context.Context, sm *StateMachine) (WorkflowState, *stageFailure) {
	if failure := sm.service.executeReview(ctx, sm.state); failure != nil {
		return nil, failure
	}
	// Terminal success condition: approved by reviewer and validation passed.
	if sm.state.review.Approved {
		return nil, nil
	}
	// Terminal threshold check: if no further repairs are available, stop.
	if !sm.state.input.RepairAvailable(sm.state.repairAttempts) {
		return nil, nil
	}
	// Transition to repair state for next attempt.
	return &RepairState{}, nil
}

// RepairState supplies blocking review findings to the implementer to repair
// the codebase, then transitions back to validation for re-checking.
type RepairState struct{}

func (s *RepairState) Stage() store.WorkflowStage {
	return store.WorkflowStageRepair
}

func (s *RepairState) Execute(ctx context.Context, sm *StateMachine) (WorkflowState, *stageFailure) {
	if failure := sm.service.executeRepair(ctx, sm.state); failure != nil {
		return nil, failure
	}
	// Re-enter the validation-review loop.
	return &ValidationState{}, nil
}
