package codex

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
)

func TestNewPlannerAppliesRecommendedDefaults(t *testing.T) {
	runner := &fakeProcessRunner{run: func(
		context.Context,
		process.Command,
	) (process.Result, error) {
		return process.Result{}, errors.New("unused")
	}}
	planner, err := NewPlanner(runner, Config{})
	if err != nil {
		t.Fatalf("NewPlanner() returned an error: %v", err)
	}

	if actual, expected := planner.executor.config, DefaultConfig(); !reflect.DeepEqual(actual, expected) {
		t.Fatalf("resolved config = %#v; want %#v", actual, expected)
	}
}

func TestNewReviewerPreservesCustomConfiguration(t *testing.T) {
	extraArguments := []string{"--search", "--enable=feature_name"}
	config := Config{
		Executable: " /opt/codex ",
		Model:      " custom-model ",
		Reasoning:  " high ",
		Timeout:    45 * time.Second,
		Sandbox:    SandboxWorkspaceWrite,
		ExtraArgs:  extraArguments,
	}
	runner := &fakeProcessRunner{run: func(
		context.Context,
		process.Command,
	) (process.Result, error) {
		return process.Result{}, errors.New("unused")
	}}
	reviewer, err := NewReviewer(runner, config)
	if err != nil {
		t.Fatalf("NewReviewer() returned an error: %v", err)
	}
	extraArguments[0] = "--mutated"

	actual := reviewer.executor.config
	if actual.Executable != "/opt/codex" || actual.Model != "custom-model" || actual.Reasoning != "high" {
		t.Fatalf("trimmed config = %#v", actual)
	}
	if actual.Timeout != 45*time.Second || actual.Sandbox != SandboxWorkspaceWrite {
		t.Fatalf("custom config = %#v", actual)
	}
	if !reflect.DeepEqual(actual.ExtraArgs, []string{"--search", "--enable=feature_name"}) {
		t.Fatalf("copied extra args = %#v", actual.ExtraArgs)
	}
}

func TestConstructorsRejectInvalidConfiguration(t *testing.T) {
	runner := &fakeProcessRunner{run: func(
		context.Context,
		process.Command,
	) (process.Result, error) {
		return process.Result{}, nil
	}}
	tests := []struct {
		name   string
		runner ProcessRunner
		config Config
		field  string
	}{
		{name: "nil runner", field: "runner"},
		{name: "invalid reasoning", runner: runner, config: Config{Reasoning: "extreme"}, field: "reasoning"},
		{name: "negative timeout", runner: runner, config: Config{Timeout: -time.Second}, field: "timeout"},
		{name: "invalid sandbox", runner: runner, config: Config{Sandbox: "networked"}, field: "sandbox"},
		{name: "positional extra argument", runner: runner, config: Config{ExtraArgs: []string{"value"}}, field: "extra_args[0]"},
		{name: "split flag value", runner: runner, config: Config{ExtraArgs: []string{"--search", "value"}}, field: "extra_args[1]"},
		{name: "whitespace in argument", runner: runner, config: Config{ExtraArgs: []string{"--feature=value with space"}}, field: "extra_args[0]"},
		{name: "managed model flag", runner: runner, config: Config{ExtraArgs: []string{"--model=other"}}, field: "extra_args[0]"},
		{name: "managed bypass flag", runner: runner, config: Config{ExtraArgs: []string{"--dangerously-bypass-approvals-and-sandbox"}}, field: "extra_args[0]"},
		{name: "managed hook bypass flag", runner: runner, config: Config{ExtraArgs: []string{"--dangerously-bypass-hook-trust"}}, field: "extra_args[0]"},
		{name: "managed rules bypass flag", runner: runner, config: Config{ExtraArgs: []string{"--ignore-rules"}}, field: "extra_args[0]"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewPlanner(test.runner, test.config)
			var configurationErr *ConfigurationError
			if !errors.As(err, &configurationErr) {
				t.Fatalf("NewPlanner() error = %v; want ConfigurationError", err)
			}
			if configurationErr.Field != test.field {
				t.Fatalf("error field = %q; want %q", configurationErr.Field, test.field)
			}
		})
	}
}
