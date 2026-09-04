package codex

import (
	"context"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// Implementer uses fresh, ephemeral workspace-write invocations. Full task,
// plan, repository and feedback replace cross-provider session assumptions.
type Implementer struct{ executor executor }

func NewImplementer(runner ProcessRunner, config Config) (*Implementer, error) {
	executor, err := newExecutor(runner, config)
	if err != nil {
		return nil, err
	}
	if executor.config.Sandbox != SandboxWorkspaceWrite {
		return nil, &ConfigurationError{Field: "sandbox", Message: "Codex implementation requires workspace-write"}
	}
	return &Implementer{executor: executor}, nil
}

func (i *Implementer) Implement(ctx context.Context, request store.ImplementationRequest) (store.ImplementationResult, error) {
	if err := request.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	prompt, err := structured.ImplementationPrompt(request)
	if err != nil {
		return store.ImplementationResult{}, err
	}
	return i.execute(ctx, "implementation", request.Input.WorkingDir, prompt)
}

func (i *Implementer) ApplyReview(ctx context.Context, request store.RepairRequest) (store.ImplementationResult, error) {
	if err := request.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	request.Implementation.AgentSessionID = ""
	prompt, err := structured.RepairPrompt(request)
	if err != nil {
		return store.ImplementationResult{}, err
	}
	return i.execute(ctx, "repair", request.Input.WorkingDir, prompt)
}

func (i *Implementer) execute(ctx context.Context, role, dir, prompt string) (store.ImplementationResult, error) {
	prompt += "\nThis may be a billing-failure handoff. Inspect partial work before continuing; do not blindly replay completed changes or external side effects. Earlier validation/findings describe the previous completed round, not proof about newer partial edits."
	data, err := i.executor.execute(ctx, role, dir, structured.ImplementationSchema(), prompt)
	if err != nil {
		return store.ImplementationResult{}, err
	}
	result, err := structured.ParseImplementation(data)
	if err != nil {
		return result, &OutputError{Role: role, Cause: err}
	}
	return result, nil
}

var _ workflow.Implementer = (*Implementer)(nil)
