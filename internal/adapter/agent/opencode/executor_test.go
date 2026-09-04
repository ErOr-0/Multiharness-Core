package opencode

import (
	"context"
	"errors"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func TestImplementPreservesCommandFailureDetailsAndObservedSession(t *testing.T) {
	sentinel := errors.New("command failed")
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		writeOutput(t, command, `{"type":"step_start","sessionID":"ses_failed","part":{"type":"step-start"}}`+"\n")
		return process.Result{Stderr: "diagnostic output", ExitCode: 7}, sentinel
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	_, err = implementer.Implement(context.Background(), validImplementationRequest(t))
	var executionErr *ExecutionError
	if !errors.As(err, &executionErr) {
		t.Fatalf("Implement() error = %v; want ExecutionError", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("Implement() error = %v; want wrapped command failure", err)
	}
	if executionErr.Operation != operationImplementation ||
		executionErr.SessionID != "ses_failed" ||
		executionErr.Stderr != "diagnostic output" {
		t.Fatalf("ExecutionError = %#v", executionErr)
	}
	if strings.Contains(err.Error(), "diagnostic output") {
		t.Fatal("ExecutionError string unexpectedly embeds command stderr")
	}
}

func TestApplyReviewPreservesCancellation(t *testing.T) {
	runner := &fakeProcessRunner{run: func(
		ctx context.Context,
		_ process.Command,
	) (process.Result, error) {
		return process.Result{}, ctx.Err()
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = implementer.ApplyReview(ctx, validRepairRequest(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ApplyReview() error = %v; want context.Canceled", err)
	}
	var executionErr *ExecutionError
	if !errors.As(err, &executionErr) ||
		executionErr.Operation != operationRepair ||
		executionErr.SessionID != "ses_original" {
		t.Fatalf("ApplyReview() error = %v; want repair ExecutionError", err)
	}
}

func TestImplementTreatsAgentErrorEventAsExecutionFailure(t *testing.T) {
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		writeOutput(t, command,
			`{"type":"error","sessionID":"ses_error","error":{"name":"ProviderError","data":{"message":"quota exhausted"}}}`+"\n",
		)
		return process.Result{ExitCode: 0}, nil
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	_, err = implementer.Implement(context.Background(), validImplementationRequest(t))
	var executionErr *ExecutionError
	if !errors.As(err, &executionErr) || executionErr.SessionID != "ses_error" {
		t.Fatalf("Implement() error = %v; want session-aware ExecutionError", err)
	}
	var eventErr *store.ProviderFailure
	if !errors.As(err, &eventErr) || eventErr.Kind != store.ProviderBillingExhausted || eventErr.Transient() {
		t.Fatalf("Implement() error = %v; want terminal billing failure", err)
	}
}

func TestImplementRejectsSuccessfulCommandWithoutCompleteEvents(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{name: "empty stream"},
		{name: "no final text", output: `{"type":"step_start","sessionID":"ses_empty","part":{"type":"step-start"}}` + "\n"},
		{name: "malformed json", output: "{bad json}\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeProcessRunner{run: func(
				_ context.Context,
				command process.Command,
			) (process.Result, error) {
				writeOutput(t, command, test.output)
				return process.Result{ExitCode: 0}, nil
			}}
			implementer, err := NewImplementer(runner, Config{}, nil)
			if err != nil {
				t.Fatalf("NewImplementer() returned an error: %v", err)
			}

			_, err = implementer.Implement(context.Background(), validImplementationRequest(t))
			var outputErr *OutputError
			if !errors.As(err, &outputErr) {
				t.Fatalf("Implement() error = %v; want OutputError", err)
			}
			if test.name == "no final text" && outputErr.SessionID != "ses_empty" {
				t.Fatalf("OutputError session = %q; want ses_empty", outputErr.SessionID)
			}
		})
	}
}
