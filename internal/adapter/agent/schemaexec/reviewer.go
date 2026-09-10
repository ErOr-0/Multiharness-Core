package schemaexec

import (
	"multiharness-core/internal/workflow"
)

// Reviewer delegates role contracts to the shared structured agent.
type Reviewer struct{ workflow.Reviewer }

func NewReviewer(runner ProcessRunner, config Config) (*Reviewer, error) {
	executor, err := newExecutor(runner, config)
	if err != nil {
		return nil, err
	}
	return &Reviewer{Reviewer: executor.agent(false)}, nil
}
