package validation

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

type runnerFunc func(context.Context, process.Command) (process.Result, error)

func (f runnerFunc) Run(ctx context.Context, command process.Command) (process.Result, error) {
	return f(ctx, command)
}

func request() store.ValidationRequest {
	return store.ValidationRequest{
		Input:          store.TaskInput{Task: "test", WorkingDir: "/workspace"},
		Plan:           store.Plan{Action: store.PlanActionImplement, Summary: "plan", Steps: []string{"step"}, AcceptanceCriteria: []string{"passes"}},
		Implementation: store.ImplementationResult{Summary: "implemented"},
	}
}

func TestValidatorCollectsFailuresThenSuccessWithBoundedOutput(t *testing.T) {
	var commands []process.Command
	runner := runnerFunc(func(_ context.Context, command process.Command) (process.Result, error) {
		commands = append(commands, command)
		if len(commands) == 1 {
			return process.Result{ExitCode: 3, Stdout: "failed", Stderr: "diagnostic", Duration: 17 * time.Millisecond},
				&process.RunError{Kind: process.ErrorKindNonZeroExit, ExitCode: 3, Cause: errors.New("exit 3")}
		}
		return process.Result{ExitCode: 0, Stdout: strings.Repeat("x", 100), StdoutTruncated: true, Duration: 20 * time.Millisecond}, nil
	})
	args := []string{"test", "./..."}
	env := map[string]string{"TEST_MODE": "yes"}
	validator, err := NewValidator(
		runner,
		Config{
			Checks:      []Check{{Executable: "go", Args: args, EnvOverrides: env}, {Executable: "go", Args: []string{"vet", "./..."}}},
			OutputLimit: 24,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	args[0] = "mutated"
	env["TEST_MODE"] = "mutated"
	report, err := validator.Validate(t.Context(), request())
	if err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
	if report.Passed || len(report.Checks) != 2 || report.Checks[0].ExitCode != 3 || !report.Checks[1].Passed {
		t.Fatalf("report: %#v", report)
	}
	if len(report.Checks[1].Output) > 24 || !report.Checks[1].OutputTruncated {
		t.Fatal("output not bounded")
	}
	if report.Checks[0].DurationMillis != 17 || report.Checks[0].Command != `"go" "test" "./..."` {
		t.Fatalf("command evidence: %#v", report.Checks[0])
	}
	if !reflect.DeepEqual(commands[0].Args, []string{"test", "./..."}) || commands[0].EnvOverrides["TEST_MODE"] != "yes" || commands[0].Dir != "/workspace" {
		t.Fatalf("command: %#v", commands[0])
	}
}

func TestValidatorStopsOnInfrastructureFailureAndReturnsEvidence(t *testing.T) {
	calls := 0
	sentinel := &process.RunError{Kind: process.ErrorKindExecutableNotFound, Cause: errors.New("not installed")}
	validator, err := NewValidator(runnerFunc(func(context.Context, process.Command) (process.Result, error) {
		calls++
		return process.Result{ExitCode: -1}, sentinel
	}),
		Config{Checks: []Check{{Executable: "missing"}, {Executable: "never"}}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := validator.Validate(t.Context(), request())
	if !errors.Is(err, sentinel) || calls != 1 || len(report.Checks) != 1 || report.Passed {
		t.Fatalf("report/error/calls: %#v %v %d", report, err, calls)
	}
	if err := report.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidatorCancellationAndInvalidRequestsDoNotRunCommands(t *testing.T) {
	validator, err := NewValidator(runnerFunc(func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("unexpected execution")
		return process.Result{}, nil
	}), Config{Checks: []Check{{Executable: "test"}}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := validator.Validate(ctx, request()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := validator.Validate(nil, request()); err == nil {
		t.Fatal("nil context accepted")
	}
	if _, err := validator.Validate(t.Context(), store.ValidationRequest{}); err == nil {
		t.Fatal("invalid request accepted")
	}
}

func TestValidatorNoChecksIsExplicitlyEmptyAndCancellationStillWins(t *testing.T) {
	validator, _ := NewValidator(runnerFunc(func(context.Context, process.Command) (process.Result, error) { panic("unused") }), Config{})
	report, err := validator.Validate(t.Context(), request())
	if err != nil || !report.Passed || len(report.Checks) != 0 {
		t.Fatalf("report: %#v %v", report, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := validator.Validate(ctx, request()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestValidatorRejectsInvalidConfiguration(t *testing.T) {
	runner := runnerFunc(func(context.Context, process.Command) (process.Result, error) { return process.Result{}, nil })
	for _, config := range []Config{
		{DefaultTimeout: -1},
		{OutputLimit: -1},
		{Checks: []Check{{}}},
		{Checks: []Check{{Executable: "x", Timeout: -1}}},
		{Checks: []Check{{Executable: "x", Args: []string{"bad\x00arg"}}}},
		{Checks: []Check{{Executable: "x", EnvOverrides: map[string]string{"BAD=KEY": "x"}}}},
	} {
		if _, err := NewValidator(runner, config); err == nil {
			t.Fatalf("invalid config accepted: %#v", config)
		}
	}
	if _, err := NewValidator(nil, Config{}); err == nil {
		t.Fatal("nil runner accepted")
	}
}
