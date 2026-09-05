// Package schemaexec adapts the Codex CLI for planning, review and fallback
// implementation. Role methods use shared structured prompts and schemas;
// command execution and runtime compatibility stay outside the workflow core.
package schemaexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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

func buildCommand(
	config Config,
	workingDir string,
	prompt string,
	artifacts invocationArtifacts,
) process.Command {
	arguments := []string{
		"exec",
		"--model", config.Model,
		"--sandbox", string(config.Sandbox),
		"--ephemeral",
		"--json",
		"--color", "never",
		"--cd", workingDir,
		"--config", "model_reasoning_effort=" + strconv.Quote(config.Reasoning),
		"--output-schema", artifacts.schemaPath,
		"--output-last-message", artifacts.outputPath,
	}
	arguments = append(arguments, config.ExtraArgs...)
	arguments = append(arguments, "-")

	return process.Command{
		Name:        config.Executable,
		Args:        arguments,
		Dir:         workingDir,
		Timeout:     config.Timeout,
		Stdin:       strings.NewReader(prompt),
		OutputLimit: process.DefaultOutputLimit,
	}
}

const maxFinalOutputBytes = 4 * 1024 * 1024

type invocationArtifacts struct {
	directory  string
	schemaPath string
	outputPath string
}

func createInvocationArtifacts(role string, schema []byte) (invocationArtifacts, error) {
	directory, err := os.MkdirTemp("", "multiharness-codex-")
	if err != nil {
		return invocationArtifacts{}, fmt.Errorf("create temporary directory: %w", err)
	}

	artifacts := invocationArtifacts{
		directory:  directory,
		schemaPath: filepath.Join(directory, role+".schema.json"),
		outputPath: filepath.Join(directory, role+".output.json"),
	}
	if err := os.WriteFile(artifacts.schemaPath, schema, 0o600); err != nil {
		artifacts.cleanup()
		return invocationArtifacts{}, fmt.Errorf("write output schema: %w", err)
	}
	if err := os.WriteFile(artifacts.outputPath, nil, 0o600); err != nil {
		artifacts.cleanup()
		return invocationArtifacts{}, fmt.Errorf("create output file: %w", err)
	}
	return artifacts, nil
}

func (artifacts invocationArtifacts) readOutput() ([]byte, error) {
	file, err := os.Open(artifacts.outputPath)
	if err != nil {
		return nil, fmt.Errorf("open final response: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxFinalOutputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read final response: %w", err)
	}
	if len(data) > maxFinalOutputBytes {
		return nil, fmt.Errorf("final response exceeds %d bytes", maxFinalOutputBytes)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("final response is blank")
	}
	return data, nil
}

func (artifacts invocationArtifacts) cleanup() {
	if artifacts.directory != "" {
		_ = os.RemoveAll(artifacts.directory)
	}
}

var (
	errNilContext = errors.New("context must not be nil")
	errNilRunner  = errors.New("process runner must not be nil")
)

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
