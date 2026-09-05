package schemaexec

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func TestPlannerPreservesCommandFailureDetails(t *testing.T) {
	sentinel := errors.New("command failed")
	runner := &fakeProcessRunner{run: func(
		context.Context,
		process.Command,
	) (process.Result, error) {
		return process.Result{Stderr: "diagnostic output", ExitCode: 7}, sentinel
	}}
	planner, err := NewPlanner(runner, Config{})
	if err != nil {
		t.Fatalf("NewPlanner() returned an error: %v", err)
	}

	_, err = planner.Plan(context.Background(), validTaskInput(t))
	var executionErr *ExecutionError
	if !errors.As(err, &executionErr) {
		t.Fatalf("Plan() error = %v; want ExecutionError", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Plan() error = %v; want wrapped command failure", err)
	}
	if executionErr.Role != rolePlanning || executionErr.Stderr != "diagnostic output" {
		t.Fatalf("ExecutionError = %#v", executionErr)
	}
	if strings.Contains(err.Error(), "diagnostic output") {
		t.Fatal("ExecutionError string unexpectedly embeds command stderr")
	}
}

func TestReviewerPreservesCancellation(t *testing.T) {
	runner := &fakeProcessRunner{run: func(
		ctx context.Context,
		_ process.Command,
	) (process.Result, error) {
		return process.Result{}, ctx.Err()
	}}
	reviewer, err := NewReviewer(runner, Config{})
	if err != nil {
		t.Fatalf("NewReviewer() returned an error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = reviewer.Review(ctx, validReviewRequest(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Review() error = %v; want context.Canceled", err)
	}
	var executionErr *ExecutionError
	if !errors.As(err, &executionErr) || executionErr.Role != roleReview {
		t.Fatalf("Review() error = %v; want review ExecutionError", err)
	}
}

func TestReadOnlyAgentsRejectInvalidFinalResponses(t *testing.T) {
	for _, role := range []string{rolePlanning, roleReview} {
		for _, response := range []string{" \n\t", "invalid JSON"} {
			t.Run(role+"/"+response, func(t *testing.T) {
				runner := &fakeProcessRunner{run: func(_ context.Context, command process.Command) (process.Result, error) {
					writeFinalResponse(t, command, response)
					return process.Result{}, nil
				}}
				planner, err := NewPlanner(runner, Config{})
				if err != nil {
					t.Fatal(err)
				}
				reviewer, err := NewReviewer(runner, Config{})
				if err != nil {
					t.Fatal(err)
				}
				if role == rolePlanning {
					_, err = planner.Plan(t.Context(), validTaskInput(t))
				} else {
					_, err = reviewer.Review(t.Context(), validReviewRequest(t))
				}
				var outputErr *OutputError
				if !errors.As(err, &outputErr) || outputErr.Role != role || runner.calls != 1 {
					t.Fatalf("invalid response lost its role or replayed: calls=%d err=%v", runner.calls, err)
				}
			})
		}
	}
}

func TestPlannerRejectsNilContext(t *testing.T) {
	runner := &fakeProcessRunner{run: func(
		context.Context,
		process.Command,
	) (process.Result, error) {
		t.Fatal("runner called with nil context")
		return process.Result{}, nil
	}}
	planner, err := NewPlanner(runner, Config{})
	if err != nil {
		t.Fatalf("NewPlanner() returned an error: %v", err)
	}

	_, err = planner.Plan(nil, validTaskInput(t))
	if !errors.Is(err, errNilContext) || runner.calls != 0 {
		t.Fatalf("Plan() error/calls = %v/%d; want nil-context error and zero calls", err, runner.calls)
	}
}

type fakeProcessRunner struct {
	run   func(context.Context, process.Command) (process.Result, error)
	calls int
}

func (runner *fakeProcessRunner) Run(
	ctx context.Context,
	command process.Command,
) (process.Result, error) {
	runner.calls++
	return runner.run(ctx, command)
}

type invocationSnapshot struct {
	name       string
	args       []string
	dir        string
	timeout    int64
	prompt     string
	schema     []byte
	schemaPath string
	outputPath string
}

func captureInvocation(t *testing.T, command process.Command) invocationSnapshot {
	t.Helper()
	prompt, err := io.ReadAll(command.Stdin)
	if err != nil {
		t.Fatalf("read command stdin: %v", err)
	}
	schemaPath := argumentValue(t, command.Args, "--output-schema")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read command schema: %v", err)
	}
	return invocationSnapshot{
		name:       command.Name,
		args:       slices.Clone(command.Args),
		dir:        command.Dir,
		timeout:    int64(command.Timeout),
		prompt:     string(prompt),
		schema:     schema,
		schemaPath: schemaPath,
		outputPath: argumentValue(t, command.Args, "--output-last-message"),
	}
}

func writeFinalResponse(t *testing.T, command process.Command, response string) {
	t.Helper()
	path := argumentValue(t, command.Args, "--output-last-message")
	if err := os.WriteFile(path, []byte(response), 0o600); err != nil {
		t.Fatalf("write fake final response: %v", err)
	}
}

func argumentValue(t *testing.T, arguments []string, name string) string {
	t.Helper()
	for index, argument := range arguments {
		if argument == name {
			if index+1 >= len(arguments) {
				t.Fatalf("argument %q has no value in %#v", name, arguments)
			}
			return arguments[index+1]
		}
	}
	t.Fatalf("argument %q not found in %#v", name, arguments)
	return ""
}

func validTaskInput(t *testing.T) store.TaskInput {
	t.Helper()
	return store.TaskInput{
		Task:              "Add a health endpoint",
		WorkingDir:        t.TempDir(),
		MaxRepairAttempts: 2,
	}
}

func validReviewRequest(t *testing.T) store.ReviewRequest {
	t.Helper()
	return store.ReviewRequest{
		Input: validTaskInput(t),
		Plan: store.Plan{
			Action:             store.PlanActionImplement,
			Summary:            "Add and verify the endpoint.",
			Steps:              []string{"Implement the handler", "Add focused tests"},
			AcceptanceCriteria: []string{"The endpoint returns 200", "Tests pass"},
		},
		Implementation: store.ImplementationResult{
			Summary:      "Implemented the endpoint and tests.",
			ChangedFiles: []string{"health.go", "health_test.go"},
		},
		Validation: store.ValidationReport{
			Passed: true,
			Checks: []store.ValidationEvidence{{
				Command:        "go test ./...",
				Passed:         true,
				ExitCode:       0,
				Output:         "ok",
				DurationMillis: 25,
			}},
		},
	}
}
