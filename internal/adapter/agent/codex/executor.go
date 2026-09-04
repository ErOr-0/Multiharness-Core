package codex

import (
	"context"
	"fmt"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/process"
)

const (
	rolePlanning = "planning"
	roleReview   = "review"
)

// ProcessRunner is the narrow process boundary consumed by the Codex adapter.
type ProcessRunner interface {
	Run(ctx context.Context, command process.Command) (process.Result, error)
}

type executor struct {
	runner ProcessRunner
	config Config
}

func newExecutor(runner ProcessRunner, config Config) (executor, error) {
	if runner == nil {
		return executor{}, &ConfigurationError{Field: "runner", Message: errNilRunner.Error()}
	}

	config = config.withDefaults()
	if err := config.Validate(); err != nil {
		return executor{}, err
	}
	return executor{runner: runner, config: config}, nil
}

func (executor executor) execute(
	ctx context.Context,
	role string,
	workingDir string,
	schema []byte,
	prompt string,
) ([]byte, error) {
	if ctx == nil {
		return nil, &ExecutionError{Role: role, Cause: errNilContext}
	}

	artifacts, err := createInvocationArtifacts(role, schema)
	if err != nil {
		return nil, &ExecutionError{Role: role, Cause: err}
	}
	defer artifacts.cleanup()

	result, err := provider.Run(
		ctx, executor.runner,
		buildCommand(executor.config, workingDir, prompt, artifacts),
	)
	if err != nil {
		return nil, &ExecutionError{Role: role, Stderr: result.Stderr, Cause: err}
	}

	output, err := artifacts.readOutput()
	if err != nil {
		return nil, &OutputError{Role: role, Cause: fmt.Errorf("read constrained response: %w", err)}
	}
	return output, nil
}
