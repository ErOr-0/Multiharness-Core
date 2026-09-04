package process

import (
	"errors"
	"fmt"
)

var (
	errInvalidCommandName = errors.New("command name must not be blank")
	errInvalidTimeout     = errors.New("command timeout must be zero or greater")
	errInvalidOutputLimit = errors.New("command output limit must be zero or greater")
	errNilContext         = errors.New("context must not be nil")
)

// ErrorKind is a stable, machine-readable process failure category.
type ErrorKind string

const (
	ErrorKindInvalidCommand     ErrorKind = "invalid_command"
	ErrorKindExecutableNotFound ErrorKind = "executable_not_found"
	ErrorKindWorkingDirectory   ErrorKind = "working_directory"
	ErrorKindStart              ErrorKind = "start_error"
	ErrorKindWait               ErrorKind = "wait_error"
	ErrorKindOutput             ErrorKind = "output_error"
	ErrorKindNonZeroExit        ErrorKind = "non_zero_exit"
	ErrorKindTimeout            ErrorKind = "timeout"
	ErrorKindCancelled          ErrorKind = "cancelled"
)

// RunError describes why an executable invocation failed without embedding
// potentially large command output. Output remains available in Result.
type RunError struct {
	Kind     ErrorKind
	Command  string
	ExitCode int
	Cause    error
}

func (err *RunError) Error() string {
	if err.Kind == ErrorKindNonZeroExit {
		return fmt.Sprintf("command %q exited with code %d: %v", err.Command, err.ExitCode, err.Cause)
	}
	return fmt.Sprintf("command %q failed (%s): %v", err.Command, err.Kind, err.Cause)
}

func (err *RunError) Unwrap() error {
	return err.Cause
}

func runError(kind ErrorKind, command string, exitCode int, cause error) *RunError {
	return &RunError{Kind: kind, Command: command, ExitCode: exitCode, Cause: cause}
}
