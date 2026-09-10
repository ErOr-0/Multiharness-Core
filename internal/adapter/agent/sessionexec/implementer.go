package sessionexec

import (
	"context"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/workflow"
)

const (
	operationImplementation = "implementation"
	operationRepair         = "repair"
)

// Implementer executes initial implementation and review-driven repairs.
type Implementer struct {
	structured.Agent
	runner ProcessRunner
	config Config
}

// NewImplementer constructs an OpenCode workflow implementer with validated
// configuration. Live activity is reported by the process runner.
func NewImplementer(
	runner ProcessRunner,
	config Config,
) (*Implementer, error) {
	if runner == nil {
		return nil, &ConfigurationError{Field: "runner", Message: errNilRunner.Error()}
	}
	config = config.withDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	a := &Implementer{runner: runner, config: config}
	a.Agent = structured.Agent{CanWrite: true, Resume: true,
		Execute: func(ctx context.Context, r structured.Invocation) (structured.Response, error) {
			if ctx == nil {
				return structured.Response{}, &ExecutionError{Operation: r.Role, Cause: errNilContext}
			}
			if err := validateSessionID(r.SessionID); err != nil {
				return structured.Response{}, &OutputError{Operation: r.Role, SessionID: r.SessionID, Cause: err}
			}
			return a.execute(ctx, r.Role, r.WorkingDir, r.SessionID, r.Prompt)
		}, OutputError: func(role, session string, err error) error {
			return &OutputError{Operation: role, SessionID: session, Cause: err}
		}}
	return a, nil
}

var _ workflow.Implementer = (*Implementer)(nil)
