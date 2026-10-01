package schemaexec

import (
	"multiharness-core/internal/adapter/agent/structured"
)

// Planner delegates role contracts to the shared structured agent.
type Planner struct{ structured.Agent }

func NewPlanner(runner ProcessRunner, config Config) (*Planner, error) {
	executor, err := newExecutor(runner, config)
	if err != nil {
		return nil, err
	}
	return &Planner{Agent: executor.agent(false)}, nil
}
