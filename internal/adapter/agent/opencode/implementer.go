package opencode

import (
	"context"
	"fmt"
	"strings"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

const (
	operationImplementation = "implementation"
	operationRepair         = "repair"
)

// ProcessRunner is the narrow process boundary consumed by the OpenCode adapter.
type ProcessRunner interface {
	Run(ctx context.Context, command process.Command) (process.Result, error)
}

// Implementer executes initial implementation and review-driven repairs.
type Implementer struct {
	runner   ProcessRunner
	config   Config
	progress ProgressSink
}

// NewImplementer constructs an OpenCode workflow implementer with validated
// configuration. A nil progress sink discards provider progress events.
func NewImplementer(
	runner ProcessRunner,
	config Config,
	progress ProgressSink,
) (*Implementer, error) {
	if runner == nil {
		return nil, &ConfigurationError{Field: "runner", Message: errNilRunner.Error()}
	}
	config = config.withDefaults()
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if progress == nil {
		progress = discardProgressSink{}
	}
	return &Implementer{runner: runner, config: config, progress: progress}, nil
}

// Implement applies the approved plan in a fresh OpenCode session.
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
	prompt, err := buildImplementationPrompt(request)
	if err != nil {
		return store.ImplementationResult{}, err
	}
	return implementer.execute(ctx, operationImplementation, request.Input.WorkingDir, "", prompt)
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
	prompt, err := buildRepairPrompt(request)
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

func (implementer *Implementer) execute(
	ctx context.Context,
	operation string,
	workingDir string,
	expectedSessionID string,
	prompt string,
) (store.ImplementationResult, error) {
	stream := newEventStream(expectedSessionID, implementer.progress)
	result, err := provider.Run(
		ctx, implementer.runner,
		buildCommand(implementer.config, workingDir, expectedSessionID, prompt, stream),
	)
	if err != nil {
		return store.ImplementationResult{}, &ExecutionError{
			Operation: operation,
			SessionID: stream.session(),
			Stderr:    result.Stderr,
			Cause:     err,
		}
	}

	events, err := stream.finish()
	if err != nil {
		return store.ImplementationResult{}, &OutputError{
			Operation: operation,
			SessionID: stream.session(),
			Cause:     err,
		}
	}
	if events.agentError != "" {
		return store.ImplementationResult{}, &ExecutionError{
			Operation: operation,
			SessionID: events.sessionID,
			Stderr:    result.Stderr,
			Cause:     &store.ProviderFailure{Kind: store.ProviderUnknown, Attempts: 1},
		}
	}
	implementation, err := parseImplementation([]byte(events.finalText), events.sessionID)
	if err != nil {
		return store.ImplementationResult{}, &OutputError{
			Operation: operation,
			SessionID: events.sessionID,
			Cause:     err,
		}
	}
	return implementation, nil
}

func parseImplementation(data []byte, sessionID string) (store.ImplementationResult, error) {
	result, err := structured.ParseImplementation(unwrapJSONFence(data))
	if err != nil {
		return store.ImplementationResult{}, err
	}
	if sessionID == "" {
		return store.ImplementationResult{}, fmt.Errorf("no OpenCode session ID was reported")
	}
	if err := validateSessionID(sessionID); err != nil {
		return store.ImplementationResult{}, err
	}
	result.AgentSessionID = sessionID
	return result, nil
}

func validateSessionID(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if strings.TrimSpace(sessionID) != sessionID || strings.ContainsAny(sessionID, "\t\r\n\x00") {
		return fmt.Errorf("must not contain surrounding whitespace, control whitespace, or NUL")
	}
	if strings.HasPrefix(sessionID, "-") {
		return fmt.Errorf("must not begin with a flag prefix")
	}
	return nil
}

var _ workflow.Implementer = (*Implementer)(nil)
