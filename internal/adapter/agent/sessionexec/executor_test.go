package sessionexec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
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
	implementer, err := NewImplementer(runner, Config{})
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
	implementer, err := NewImplementer(runner, Config{})
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
		writeOutput(
			t,
			command,
			`{"type":"error","sessionID":"ses_error","error":{"name":"ProviderError","data":{"message":"quota exhausted"}}}`+"\n",
		)
		return process.Result{ExitCode: 0}, nil
	}}
	implementer, err := NewImplementer(runner, Config{})
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
			implementer, err := NewImplementer(runner, Config{})
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
	name    string
	args    []string
	dir     string
	timeout int64
	prompt  string
}

func captureInvocation(t *testing.T, command process.Command) invocationSnapshot {
	t.Helper()
	prompt, err := io.ReadAll(command.Stdin)
	if err != nil {
		t.Fatalf("read command stdin: %v", err)
	}
	return invocationSnapshot{
		name:    command.Name,
		args:    slices.Clone(command.Args),
		dir:     command.Dir,
		timeout: int64(command.Timeout),
		prompt:  string(prompt),
	}
}

func writeOutput(t *testing.T, command process.Command, chunks ...string) {
	t.Helper()
	if command.Stdout == nil {
		t.Fatal("command stdout is nil")
	}
	for _, chunk := range chunks {
		if _, err := io.WriteString(command.Stdout, chunk); err != nil {
			t.Fatalf("write command stdout: %v", err)
		}
	}
}

func validImplementationRequest(t *testing.T) store.ImplementationRequest {
	t.Helper()
	return store.ImplementationRequest{
		Input: store.TaskInput{
			Task:              "Add a health endpoint",
			WorkingDir:        t.TempDir(),
			MaxRepairAttempts: 2,
		},
		Plan: store.Plan{
			Action:             store.PlanActionImplement,
			Summary:            "Add and verify the endpoint.",
			Steps:              []string{"Implement the handler", "Add focused tests"},
			AcceptanceCriteria: []string{"The endpoint returns 200", "Tests pass"},
		},
	}
}

func validRepairRequest(t *testing.T) store.RepairRequest {
	t.Helper()
	implementation := validImplementationRequest(t)
	return store.RepairRequest{
		Input: implementation.Input,
		Plan:  implementation.Plan,
		Implementation: store.ImplementationResult{
			Summary:        "Implemented the endpoint.",
			ChangedFiles:   []string{"health.go"},
			AgentSessionID: "ses_original",
		},
		Validation: store.ValidationReport{
			Passed: false,
			Checks: []store.ValidationEvidence{{
				Command:        "go test ./...",
				Passed:         false,
				ExitCode:       1,
				Output:         "health_test.go:42: expected 200",
				DurationMillis: 28,
			}},
		},
		Review: store.Review{
			Approved: false,
			Summary:  "The response status is wrong.",
			Findings: []store.ReviewFinding{
				{
					Severity:       store.FindingSeverityError,
					Blocking:       true,
					File:           "health.go",
					Line:           12,
					Description:    "Handler returns 204.",
					Evidence:       "The failing test observed 204.",
					RequiredAction: "Return 200.",
				},
				{
					Severity:    store.FindingSeverityInfo,
					Blocking:    false,
					Description: "Consider a helper name change.",
				},
			},
			Suggestions: []string{"Rename the helper later."},
		},
	}
}

func successfulEventStream(sessionID, summary string, changedFiles ...string) string {
	response, _ := json.Marshal(map[string]any{
		"schema_version": "1",
		"summary":        summary,
		"changed_files":  changedFiles,
	})
	textEvent, _ := json.Marshal(map[string]any{
		"type":      "text",
		"sessionID": sessionID,
		"part": map[string]any{
			"type": "text",
			"text": string(response),
		},
	})
	return `{"type":"step_start","sessionID":"` + sessionID + `","part":{"type":"step-start"}}` + "\n" +
		string(textEvent) + "\n" +
		`{"type":"step_finish","sessionID":"` + sessionID + `","part":{"type":"step-finish"}}` + "\n"
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
