package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

func contextError(command string, err error) *RunError {
	if errors.Is(err, context.DeadlineExceeded) {
		return runError(ErrorKindTimeout, command, -1, context.DeadlineExceeded)
	}
	return runError(ErrorKindCancelled, command, -1, context.Canceled)
}

func classifyStartError(command Command, err error) *RunError {
	var execError *exec.Error
	if errors.As(err, &execError) || errors.Is(err, exec.ErrNotFound) {
		return runError(ErrorKindExecutableNotFound, command.Name, -1, err)
	}

	var pathError *os.PathError
	if errors.As(err, &pathError) {
		if pathError.Op == "chdir" {
			return runError(ErrorKindWorkingDirectory, command.Name, -1, err)
		}
		if errors.Is(err, os.ErrNotExist) {
			return runError(ErrorKindExecutableNotFound, command.Name, -1, err)
		}
	}

	return runError(ErrorKindStart, command.Name, -1, fmt.Errorf("start process: %w", err))
}

func classifyWaitError(
	command string,
	exitCode int,
	waitErr error,
	ctx context.Context,
	outputErrors ...error,
) *RunError {
	if err := ctx.Err(); err != nil {
		return contextError(command, err)
	}
	if outputErr := firstError(outputErrors...); outputErr != nil {
		return runError(ErrorKindOutput, command, exitCode, outputErr)
	}

	var exitError *exec.ExitError
	if errors.As(waitErr, &exitError) {
		return runError(ErrorKindNonZeroExit, command, exitCode, waitErr)
	}
	return runError(ErrorKindWait, command, exitCode, waitErr)
}

func firstError(errors ...error) error {
	for _, err := range errors {
		if err != nil {
			return err
		}
	}
	return nil
}
