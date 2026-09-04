package workflow

import "multiharness-core/internal/store"

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
