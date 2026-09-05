// Package sessionexec implements the OpenCode CLI protocol for planning, implementation,
// review and repair. Provider events and CLI details remain outside the workflow.
package sessionexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

var (
	errNilContext = errors.New("context must not be nil")
	errNilRunner  = errors.New("process runner must not be nil")
)

// ProcessRunner is the narrow process boundary consumed by the OpenCode adapter.
type ProcessRunner interface {
	Run(ctx context.Context, command process.Command) (process.Result, error)
}

// ExecutionError reports an OpenCode invocation failure without embedding the
// prompt or captured stderr in its printable form.
type ExecutionError struct {
	Operation string
	SessionID string
	Stderr    string
	Cause     error
}

func (err *ExecutionError) Error() string {
	return fmt.Sprintf("execute OpenCode %s: %v", err.Operation, err.Cause)
}

func (err *ExecutionError) Unwrap() error {
	return err.Cause
}

// OutputError identifies malformed, incomplete, or inconsistent OpenCode JSON
// event output.
type OutputError struct {
	Operation string
	SessionID string
	Cause     error
}

func (err *OutputError) Error() string {
	return fmt.Sprintf("invalid OpenCode %s output: %v", err.Operation, err.Cause)
}

func (err *OutputError) Unwrap() error {
	return err.Cause
}

func buildCommand(
	config Config,
	workingDir string,
	sessionID string,
	prompt string,
	stdout io.Writer,
) process.Command {
	arguments := []string{"run", "--format", "json", "--dir", workingDir}
	if config.Model != "" {
		arguments = append(arguments, "--model", config.Model)
	}
	if config.Variant != "" {
		arguments = append(arguments, "--variant", config.Variant)
	}
	if sessionID != "" {
		arguments = append(arguments, "--session", sessionID)
	}
	if config.PermissionPolicy == PermissionAutoApprove {
		arguments = append(arguments, "--auto")
	}
	arguments = append(arguments, config.ExtraArgs...)

	return process.Command{
		Name:        config.Executable,
		Args:        arguments,
		Dir:         workingDir,
		Timeout:     config.Timeout,
		Stdin:       strings.NewReader(prompt),
		Stdout:      stdout,
		OutputLimit: process.DefaultOutputLimit,
	}
}

func (implementer *Implementer) execute(
	ctx context.Context,
	operation string,
	workingDir string,
	expectedSessionID string,
	prompt string,
) (store.ImplementationResult, error) {
	stream := newEventStream(expectedSessionID)
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
	if events.agentFailed {
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
