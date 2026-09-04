package workflow

import (
	"context"

	"multiharness-core/internal/store"
)

// StateMachine coordinates the progression of a task through its lifecycle states.
// It manages state transitions, bounds context cancellation checks, and delegates
// System I/O to the mediator.
type StateMachine struct {
	service      *Service
	state        *runState
	mediator     SystemIOMediator
	currentState WorkflowState
}

// NewStateMachine constructs a state machine initialized at the IntakeState.
func NewStateMachine(service *Service, state *runState, mediator SystemIOMediator) *StateMachine {
	return &StateMachine{
		service:      service,
		state:        state,
		mediator:     mediator,
		currentState: &IntakeState{},
	}
}

// Run executes state transitions sequentially until a terminal state is reached
// or an execution failure occurs.
func (sm *StateMachine) Run(ctx context.Context) *stageFailure {
	for sm.currentState != nil {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return failureAt(sm.currentState.Stage(), store.FailureCodeInternal, err, sm.state.repairAttempts)
			}
		}
		nextState, failure := sm.currentState.Execute(ctx, sm)
		if failure != nil {
			return failure
		}
		sm.currentState = nextState
	}
	return nil
}
