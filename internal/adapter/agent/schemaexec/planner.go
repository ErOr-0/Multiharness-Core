package schemaexec

import (
	"context"

	"multiharness-core/internal/adapter/agent/structured"
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
	prompt, err := structured.PlanningPrompt(input)
	if err != nil {
		return store.Plan{}, err
	}
	output, err := planner.executor.execute(
		ctx,
		rolePlanning,
		input.WorkingDir,
		structured.PlanSchema(),
		prompt,
	)
	if err != nil {
		return store.Plan{}, err
	}
	plan, err := structured.ParsePlan(output)
	if err != nil {
		return plan, &OutputError{Role: rolePlanning, Cause: err}
	}
	return plan, nil
}

var _ workflow.Planner = (*Planner)(nil)
