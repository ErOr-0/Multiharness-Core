package process

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

const defaultWaitDelay = 2 * time.Second

// OSRunner executes commands through the operating system.
type OSRunner struct{}

// NewOSRunner constructs a process runner with safe output and cancellation
// defaults.
func NewOSRunner() OSRunner {
	return OSRunner{}
}

// Run executes command and returns bounded output even when execution fails.
func (OSRunner) Run(ctx context.Context, command Command) (Result, error) {
	result := Result{ExitCode: -1}
	if ctx == nil {
		return result, runError(ErrorKindInvalidCommand, command.Name, -1, errNilContext)
	}
	if err := command.validate(); err != nil {
		return result, runError(ErrorKindInvalidCommand, command.Name, -1, err)
	}
	if err := validateWorkingDirectory(command.Dir); err != nil {
		return result, runError(ErrorKindWorkingDirectory, command.Name, -1, err)
	}

	runContext := ctx
	if command.Timeout > 0 {
		var cancel context.CancelFunc
		runContext, cancel = context.WithTimeout(ctx, command.Timeout)
		defer cancel()
	}
	if err := runContext.Err(); err != nil {
		return result, contextError(command.Name, err)
	}

	cmd := exec.CommandContext(runContext, command.Name, command.Args...)
	cmd.Dir = command.Dir
	baseEnvironment, err := removeEnvironment(cmd.Environ(), command.EnvUnset)
	if err != nil {
		return result, runError(ErrorKindInvalidCommand, command.Name, -1, err)
	}
	environment, err := mergeEnvironment(baseEnvironment, command.EnvOverrides)
	if err != nil {
		return result, runError(ErrorKindInvalidCommand, command.Name, -1, err)
	}
	cmd.Env = environment
	cmd.Stdin = command.Stdin
	cmd.WaitDelay = defaultWaitDelay
	configureProcessTree(cmd)

	limit := command.outputLimit()
	stdoutCapture := newTailBuffer(limit)
	stderrCapture := newTailBuffer(limit)
	outputMutex := &sync.Mutex{}
	stdoutWriter := &outputWriter{mu: outputMutex, capture: stdoutCapture, sink: command.Stdout}
	stderrWriter := &outputWriter{mu: outputMutex, capture: stderrCapture, sink: command.Stderr}
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter

	startedAt := time.Now()
	if err := cmd.Start(); err != nil {
		result.Duration = time.Since(startedAt)
		populateOutput(&result, stdoutCapture, stderrCapture)
		return result, classifyStartError(command, err)
	}

	waitErr := cmd.Wait()
	result.Duration = time.Since(startedAt)
	populateOutput(&result, stdoutCapture, stderrCapture)
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if waitErr == nil {
		return result, nil
	}
	return result, classifyWaitError(
		command.Name,
		result.ExitCode,
		waitErr,
		runContext,
		stdoutWriter.Error(),
		stderrWriter.Error(),
	)
}
