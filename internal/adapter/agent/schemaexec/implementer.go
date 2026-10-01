package schemaexec

import (
	"multiharness-core/internal/adapter/agent/structured"
)

// Implementer delegates role contracts to the shared structured agent.
type Implementer struct{ structured.Agent }

func NewImplementer(runner ProcessRunner, config Config) (*Implementer, error) {
	executor, err := newExecutor(runner, config)
	if err != nil {
		return nil, err
	}
	if executor.config.Sandbox != SandboxWorkspaceWrite {
		return nil, &ConfigurationError{Field: "sandbox", Message: "Codex implementation requires workspace-write"}
	}
	return &Implementer{Agent: executor.agent(true)}, nil
}
