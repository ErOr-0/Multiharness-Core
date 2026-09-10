package schemaexec

import (
	"multiharness-core/internal/workflow"
)

// Implementer delegates role contracts to the shared structured agent.
type Implementer struct{ workflow.Implementer }

func NewImplementer(runner ProcessRunner, config Config) (*Implementer, error) {
	executor, err := newExecutor(runner, config)
	if err != nil {
		return nil, err
	}
	if executor.config.Sandbox != SandboxWorkspaceWrite {
		return nil, &ConfigurationError{Field: "sandbox", Message: "Codex implementation requires workspace-write"}
	}
	return &Implementer{Implementer: executor.agent(true)}, nil
}
