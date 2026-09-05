package schemaexec

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func TestPlannerBuildsConstrainedCodexCommand(t *testing.T) {
	input := validTaskInput(t)
	var captured invocationSnapshot
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		captured = captureInvocation(t, command)
		writeFinalResponse(t, command, `{
			"schema_version":"2", "action":"implement", "answer":"",
			"summary":"Add the endpoint with focused tests.",
			"steps":["Add the handler","Add tests"],
			"acceptance_criteria":["Endpoint returns 200","Tests pass"]
		}`)
		return process.Result{ExitCode: 0}, nil
	}}

	planner, err := NewPlanner(runner, Config{})
	if err != nil {
		t.Fatalf("NewPlanner() returned an error: %v", err)
	}
	plan, err := planner.Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("Plan() returned an error: %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("Plan() returned an invalid plan: %v", err)
	}

	if captured.name != DefaultExecutable || captured.dir != input.WorkingDir {
		t.Fatalf("command name/dir = %q/%q", captured.name, captured.dir)
	}
	if time.Duration(captured.timeout) != DefaultTimeout {
		t.Fatalf("command timeout = %s; want %s", time.Duration(captured.timeout), DefaultTimeout)
	}
	if !strings.Contains(captured.prompt, input.Task) || !strings.Contains(captured.prompt, "planning mode only") {
		t.Fatalf("planning prompt = %q", captured.prompt)
	}
	encoded, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(captured.prompt, string(encoded)) {
		t.Fatal("planner prompt does not contain the complete task input")
	}
	if !reflect.DeepEqual(captured.schema, structured.PlanSchema()) {
		t.Fatal("command did not receive the embedded plan schema")
	}

	expectedArguments := []string{
		"exec",
		"--model", DefaultModel,
		"--sandbox", string(SandboxReadOnly),
		"--ephemeral",
		"--json",
		"--color", "never",
		"--cd", input.WorkingDir,
		"--config", `model_reasoning_effort="xhigh"`,
		"--output-schema", captured.schemaPath,
		"--output-last-message", captured.outputPath,
		"-",
	}
	if !reflect.DeepEqual(captured.args, expectedArguments) {
		t.Fatalf("command args = %#v; want %#v", captured.args, expectedArguments)
	}
}

func TestPlannerUsesConfigurationOverrides(t *testing.T) {
	input := validTaskInput(t)
	var captured invocationSnapshot
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		captured = captureInvocation(t, command)
		writeFinalResponse(t, command, `{
			"schema_version":"2", "action":"implement", "answer":"",
			"summary":"Plan",
			"steps":["Step"],
			"acceptance_criteria":["Criterion"]
		}`)
		return process.Result{}, nil
	}}
	config := Config{
		Executable: "/opt/bin/codex-custom",
		Model:      "gpt-custom",
		Reasoning:  "high",
		Timeout:    75 * time.Second,
		Sandbox:    SandboxWorkspaceWrite,
		ExtraArgs:  []string{"--search", "--enable=feature_name"},
	}
	planner, err := NewPlanner(runner, config)
	if err != nil {
		t.Fatalf("NewPlanner() returned an error: %v", err)
	}
	if _, err := planner.Plan(context.Background(), input); err != nil {
		t.Fatalf("Plan() returned an error: %v", err)
	}

	if captured.name != config.Executable || time.Duration(captured.timeout) != config.Timeout {
		t.Fatalf("command executable/timeout = %q/%s", captured.name, time.Duration(captured.timeout))
	}
	for _, expected := range []string{
		config.Model,
		string(config.Sandbox),
		`model_reasoning_effort="high"`,
		"--search",
		"--enable=feature_name",
	} {
		if !slices.Contains(captured.args, expected) {
			t.Errorf("command args %#v do not contain %q", captured.args, expected)
		}
	}
}

func TestPlannerRejectsInvalidInputBeforeExecution(t *testing.T) {
	runner := &fakeProcessRunner{run: func(
		context.Context,
		process.Command,
	) (process.Result, error) {
		t.Fatal("runner called for invalid input")
		return process.Result{}, nil
	}}
	planner, err := NewPlanner(runner, Config{})
	if err != nil {
		t.Fatalf("NewPlanner() returned an error: %v", err)
	}

	_, err = planner.Plan(context.Background(), store.TaskInput{})
	if err == nil || runner.calls != 0 {
		t.Fatalf("Plan() error/calls = %v/%d; want validation error and zero calls", err, runner.calls)
	}
}
