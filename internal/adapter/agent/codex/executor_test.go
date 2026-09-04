package codex

import (
	"context"
	"errors"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
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

func TestPlannerRejectsBlankFinalResponse(t *testing.T) {
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		writeFinalResponse(t, command, " \n\t")
		return process.Result{}, nil
	}}
	planner, err := NewPlanner(runner, Config{})
	if err != nil {
		t.Fatalf("NewPlanner() returned an error: %v", err)
	}

	_, err = planner.Plan(context.Background(), validTaskInput(t))
	var outputErr *OutputError
	if !errors.As(err, &outputErr) || outputErr.Role != rolePlanning {
		t.Fatalf("Plan() error = %v; want planning OutputError", err)
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
