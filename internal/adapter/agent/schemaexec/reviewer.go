package schemaexec

import (
	"multiharness-core/internal/adapter/agent/structured"
)

// Reviewer delegates role contracts to the shared structured agent.
type Reviewer struct{ structured.Agent }

func NewReviewer(runner ProcessRunner, config Config) (*Reviewer, error) {
	executor, err := newExecutor(runner, config)
	if err != nil {
		return nil, err
	}
	return &Reviewer{Agent: executor.agent(false)}, nil
}
