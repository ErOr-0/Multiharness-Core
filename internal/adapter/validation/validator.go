// Package validation executes configured deterministic checks independently of
// agent summaries. Commands are direct argv invocations, never implicit shells.
package validation

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type ProcessRunner interface {
	Run(context.Context, process.Command) (process.Result, error)
}

type Check struct {
	Executable   string
	Args         []string
	Timeout      time.Duration
	EnvOverrides map[string]string
}

type Config struct {
	Checks         []Check
	DefaultTimeout time.Duration
	OutputLimit    int
}

type Validator struct {
	runner ProcessRunner
	config Config
}

func NewValidator(runner ProcessRunner, config Config) (*Validator, error) {
	if runner == nil {
		return nil, fmt.Errorf("validation process runner is required")
	}
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 5 * time.Minute
	}
	if config.OutputLimit == 0 {
		config.OutputLimit = 64 << 10
	}
	if config.DefaultTimeout < 0 || config.OutputLimit < 1 {
		return nil, fmt.Errorf("validation timeout and output limit must be positive")
	}
	config.Checks = slices.Clone(config.Checks)
	for i := range config.Checks {
		check := &config.Checks[i]
		if strings.TrimSpace(check.Executable) == "" || strings.ContainsRune(check.Executable, 0) {
			return nil, fmt.Errorf("check %d executable is invalid", i)
		}
		if check.Timeout == 0 {
			check.Timeout = config.DefaultTimeout
		}
		if check.Timeout < 0 {
			return nil, fmt.Errorf("check %d timeout must be positive", i)
		}
		for _, arg := range check.Args {
			if strings.ContainsRune(arg, 0) {
				return nil, fmt.Errorf("check %d argument contains NUL", i)
			}
		}
		for key, value := range check.EnvOverrides {
			if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, 0) {
				return nil, fmt.Errorf("check %d has invalid environment override", i)
			}
		}
		check.Args = slices.Clone(check.Args)
		check.EnvOverrides = maps.Clone(check.EnvOverrides)
	}
	return &Validator{runner: runner, config: config}, nil
}

// Validate continues after ordinary test failures to collect all checks. A
// process/infrastructure error stops execution and returns the evidence gathered
// so far alongside an inspectable error. No automatic command retries occur.
func (validator *Validator) Validate(ctx context.Context, request store.ValidationRequest) (store.ValidationReport, error) {
	report := store.ValidationReport{Passed: true, Checks: []store.ValidationEvidence{}}
	if ctx == nil {
		return report, fmt.Errorf("validation context is required")
	}
	if err := request.Validate(); err != nil {
		return report, err
	}
	for _, check := range validator.config.Checks {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		result, err := validator.runner.Run(ctx, process.Command{
			Name: check.Executable, Args: slices.Clone(check.Args), Dir: request.Input.WorkingDir,
			Timeout: check.Timeout, EnvOverrides: maps.Clone(check.EnvOverrides), OutputLimit: validator.config.OutputLimit,
		})
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		exit := result.ExitCode
		if err != nil && exit == 0 {
			exit = -1
		}
		output := result.Stdout
		if result.Stderr != "" {
			output += "\n[stderr]\n" + result.Stderr
		}
		truncated := result.StdoutTruncated || result.StderrTruncated || len(output) > validator.config.OutputLimit
		if len(output) > validator.config.OutputLimit {
			output = output[len(output)-validator.config.OutputLimit:]
		}
		passed := err == nil && exit == 0
		report.Checks = append(report.Checks, store.ValidationEvidence{
			Command: displayCommand(check), Passed: passed, ExitCode: exit, Output: output,
			DurationMillis: result.Duration.Milliseconds(), OutputTruncated: truncated,
		})
		report.Passed = report.Passed && passed
		if err != nil {
			var runErr *process.RunError
			if !errors.As(err, &runErr) || runErr.Kind != process.ErrorKindNonZeroExit {
				return report, fmt.Errorf("validation check %s: %w", displayCommand(check), err)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	return report, nil
}

func displayCommand(check Check) string {
	parts := []string{strconv.Quote(check.Executable)}
	for _, arg := range check.Args {
		parts = append(parts, strconv.Quote(arg))
	}
	return strings.Join(parts, " ")
}

var _ workflow.Validator = (*Validator)(nil)
