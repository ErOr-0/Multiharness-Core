package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

type Runner interface {
	Run(ctx context.Context, command Command) (Result, error)
}

type Command struct {
	Name    string
	Args    []string
	Dir     string
	Timeout time.Duration
}

type Result struct {
	Output   string
	ExitCode int
}

type OSRunner struct{}

func (OSRunner) Run(ctx context.Context, command Command) (Result, error) {
	if command.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, command.Timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, command.Name, command.Args...)

	if command.Dir != "" {
		cmd.Dir = command.Dir
	}

	output, err := cmd.CombinedOutput()

	result := Result{
		Output:   string(output),
		ExitCode: 0,
	}

	if err == nil {
		return result, nil
	}

	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.ExitCode = exitError.ExitCode()
	} else {
		result.ExitCode = -1
	}

	if ctx.Err() != nil {
		return result, fmt.Errorf("command %q: %w", command.Name, ctx.Err())
	}

	return result, fmt.Errorf("command %q failed: %w", command.Name, err)
}
