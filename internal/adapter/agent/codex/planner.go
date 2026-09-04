package codex

import (
	"context"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// Planner invokes Codex in a schema-constrained planning role.
type Planner struct {
	executor executor
}

// NewPlanner constructs a Codex workflow planner with validated configuration.
func NewPlanner(runner ProcessRunner, config Config) (*Planner, error) {
	executor, err := newExecutor(runner, config)
	if err != nil {
		return nil, err
	}
	return &Planner{executor: executor}, nil
}

// Plan produces and validates one provider-independent workflow plan.
func (planner *Planner) Plan(ctx context.Context, input store.TaskInput) (store.Plan, error) {
	if err := input.Validate(); err != nil {
		return store.Plan{}, err
	}
	prompt, err := buildPlannerPrompt(input)
	if err != nil {
		return store.Plan{}, err
	}
	output, err := planner.executor.execute(
		ctx,
		rolePlanning,
		input.WorkingDir,
		planSchema,
		prompt,
	)
	if err != nil {
		return store.Plan{}, err
	}
	return parsePlan(output)
}

var _ workflow.Planner = (*Planner)(nil)
