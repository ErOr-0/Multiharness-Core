package sessionexec

import (
	"context"
	"fmt"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

const (
	operationImplementation = "implementation"
	operationRepair         = "repair"
)

// Implementer executes initial implementation and review-driven repairs.
type Implementer struct {
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
	return &Implementer{runner: runner, config: config}, nil
}

// Implement applies the approved plan in an OpenCode session. If request.Input.SessionID
// is set, it resumes that existing session; otherwise it starts a fresh session.
func (implementer *Implementer) Implement(
	ctx context.Context,
	request store.ImplementationRequest,
) (store.ImplementationResult, error) {
	if ctx == nil {
		return store.ImplementationResult{}, &ExecutionError{
			Operation: operationImplementation,
			Cause:     errNilContext,
		}
	}
	if err := request.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	prompt, err := structured.ImplementationPrompt(request)
	if err != nil {
		return store.ImplementationResult{}, err
	}
	sessionID := request.Input.SessionID
	if sessionID != "" {
		if err := validateSessionID(sessionID); err != nil {
			return store.ImplementationResult{}, &OutputError{
				Operation: operationImplementation,
				SessionID: sessionID,
				Cause:     fmt.Errorf("invalid initial OpenCode session ID: %w", err),
			}
		}
	}
	return implementer.execute(ctx, operationImplementation, request.Input.WorkingDir, sessionID, prompt)
}

// ApplyReview fixes a rejected review. It resumes the previous OpenCode session
// when the implementation evidence contains one, otherwise it safely starts a
// fresh session with the complete repair context.
func (implementer *Implementer) ApplyReview(
	ctx context.Context,
	request store.RepairRequest,
) (store.ImplementationResult, error) {
	if ctx == nil {
		return store.ImplementationResult{}, &ExecutionError{
			Operation: operationRepair,
			Cause:     errNilContext,
		}
	}
	if err := request.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	sessionID := request.Implementation.AgentSessionID
	if err := validateSessionID(sessionID); err != nil {
		return store.ImplementationResult{}, &OutputError{
			Operation: operationRepair,
			SessionID: sessionID,
			Cause:     fmt.Errorf("invalid prior OpenCode session ID: %w", err),
		}
	}
	prompt, err := structured.RepairPrompt(request)
	if err != nil {
		return store.ImplementationResult{}, err
	}
	return implementer.execute(
		ctx,
		operationRepair,
		request.Input.WorkingDir,
		sessionID,
		prompt,
	)
}

var _ workflow.Implementer = (*Implementer)(nil)
