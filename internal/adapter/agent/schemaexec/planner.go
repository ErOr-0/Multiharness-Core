package schemaexec

import (
	"multiharness-core/internal/workflow"
)

// Planner delegates role contracts to the shared structured agent.
type Planner struct{ workflow.Planner }

func NewPlanner(runner ProcessRunner, config Config) (*Planner, error) {
	executor, err := newExecutor(runner, config)
	if err != nil {
		return nil, err
	}
	return &Planner{Planner: executor.agent(false)}, nil
}
