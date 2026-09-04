package codex

import (
	"context"
	"io"
	"os"
	"slices"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

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
