package schemaexec

import (
	"context"
	"errors"

	"multiharness-core/internal/adapter/agent/structured"
)

// Claude supplies print-mode command/envelope details to shared schema roles.
type Claude struct {
	structured.Agent
	runner ProcessRunner
	config ClaudeConfig
}

func NewClaude(runner ProcessRunner, cfg ClaudeConfig) (*Claude, error) {
	if runner == nil {
		return nil, errors.New("Claude runner must not be nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	a := &Claude{runner: runner, config: cfg}
	a.Agent = structured.Agent{CanWrite: cfg.CanWrite, Execute: func(ctx context.Context, r structured.Invocation) (structured.Response, error) {
		data, err := a.execute(ctx, r.WorkingDir, r.Prompt, r.Schema)
		return structured.Response{Data: data}, err
	}}
	return a, nil
}
