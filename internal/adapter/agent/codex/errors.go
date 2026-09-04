package codex

import (
	"errors"
	"fmt"
)

var (
	errNilContext = errors.New("context must not be nil")
	errNilRunner  = errors.New("process runner must not be nil")
)

// ConfigurationError identifies one invalid Codex adapter setting.
type ConfigurationError struct {
	Field   string
	Message string
}

func (err *ConfigurationError) Error() string {
	return fmt.Sprintf("invalid Codex configuration %s: %s", err.Field, err.Message)
}

// ExecutionError reports a Codex command failure without embedding the prompt.
type ExecutionError struct {
	Role   string
	Stderr string
	Cause  error
}

func (err *ExecutionError) Error() string {
	return fmt.Sprintf("execute Codex %s: %v", err.Role, err.Cause)
}

func (err *ExecutionError) Unwrap() error {
	return err.Cause
}

// OutputError identifies an unreadable or invalid structured Codex response.
type OutputError struct {
	Role  string
	Cause error
}

func (err *OutputError) Error() string {
	return fmt.Sprintf("invalid Codex %s output: %v", err.Role, err.Cause)
}

func (err *OutputError) Unwrap() error {
	return err.Cause
}
