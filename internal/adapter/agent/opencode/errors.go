package opencode

import (
	"errors"
	"fmt"
)

var (
	errNilContext = errors.New("context must not be nil")
	errNilRunner  = errors.New("process runner must not be nil")
)

// ConfigurationError identifies one invalid OpenCode adapter setting.
type ConfigurationError struct {
	Field   string
	Message string
}

func (err *ConfigurationError) Error() string {
	return fmt.Sprintf("invalid OpenCode configuration %s: %s", err.Field, err.Message)
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
