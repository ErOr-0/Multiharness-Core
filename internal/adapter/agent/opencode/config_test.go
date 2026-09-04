package opencode

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
)

func TestNewImplementerAppliesSafeDefaults(t *testing.T) {
	implementer, err := NewImplementer(unusedRunner(), Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}
	if actual, expected := implementer.config, DefaultConfig(); !reflect.DeepEqual(actual, expected) {
		t.Fatalf("resolved config = %#v; want %#v", actual, expected)
	}
	if _, ok := implementer.progress.(discardProgressSink); !ok {
		t.Fatalf("default progress sink = %T; want discardProgressSink", implementer.progress)
	}
}

func TestNewImplementerPreservesCustomConfiguration(t *testing.T) {
	extraArguments := []string{"--pure", "--log-level=INFO"}
	progress := &recordingProgressSink{}
	config := Config{
		Executable:       " /opt/opencode ",
		Model:            " openai/gpt-custom ",
		Variant:          " high ",
		Timeout:          45 * time.Minute,
		PermissionPolicy: PermissionAutoApprove,
		ExtraArgs:        extraArguments,
	}
	implementer, err := NewImplementer(unusedRunner(), config, progress)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}
	extraArguments[0] = "--mutated"

	actual := implementer.config
	if actual.Executable != "/opt/opencode" || actual.Model != "openai/gpt-custom" || actual.Variant != "high" {
		t.Fatalf("trimmed config = %#v", actual)
	}
	if actual.Timeout != config.Timeout || actual.PermissionPolicy != PermissionAutoApprove {
		t.Fatalf("custom config = %#v", actual)
	}
	if !reflect.DeepEqual(actual.ExtraArgs, []string{"--pure", "--log-level=INFO"}) {
		t.Fatalf("copied extra args = %#v", actual.ExtraArgs)
	}
	if implementer.progress != progress {
		t.Fatal("custom progress sink was not preserved")
	}
}

func TestNewImplementerRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		runner ProcessRunner
		config Config
		field  string
	}{
		{name: "nil runner", field: "runner"},
		{name: "model without provider", runner: unusedRunner(), config: Config{Model: "gpt"}, field: "model"},
		{name: "model without id", runner: unusedRunner(), config: Config{Model: "openai/"}, field: "model"},
		{name: "model ending in slash", runner: unusedRunner(), config: Config{Model: "openai/models/"}, field: "model"},
		{name: "model with whitespace", runner: unusedRunner(), config: Config{Model: "openai/gpt custom"}, field: "model"},
		{name: "flag-like model", runner: unusedRunner(), config: Config{Model: "--openai/gpt"}, field: "model"},
		{name: "variant with whitespace", runner: unusedRunner(), config: Config{Variant: "very high"}, field: "variant"},
		{name: "flag-like variant", runner: unusedRunner(), config: Config{Variant: "--high"}, field: "variant"},
		{name: "negative timeout", runner: unusedRunner(), config: Config{Timeout: -time.Second}, field: "timeout"},
		{name: "invalid permission policy", runner: unusedRunner(), config: Config{PermissionPolicy: "prompt"}, field: "permission_policy"},
		{name: "positional extra argument", runner: unusedRunner(), config: Config{ExtraArgs: []string{"value"}}, field: "extra_args[0]"},
		{name: "separator extra argument", runner: unusedRunner(), config: Config{ExtraArgs: []string{"--"}}, field: "extra_args[0]"},
		{name: "split flag value", runner: unusedRunner(), config: Config{ExtraArgs: []string{"--log-level", "INFO"}}, field: "extra_args[1]"},
		{name: "whitespace in argument", runner: unusedRunner(), config: Config{ExtraArgs: []string{"--log-level= INFO"}}, field: "extra_args[0]"},
		{name: "managed model", runner: unusedRunner(), config: Config{ExtraArgs: []string{"--model=other/model"}}, field: "extra_args[0]"},
		{name: "managed attached short model", runner: unusedRunner(), config: Config{ExtraArgs: []string{"-mother/model"}}, field: "extra_args[0]"},
		{name: "managed session", runner: unusedRunner(), config: Config{ExtraArgs: []string{"--session=ses_other"}}, field: "extra_args[0]"},
		{name: "managed auto", runner: unusedRunner(), config: Config{ExtraArgs: []string{"--auto"}}, field: "extra_args[0]"},
		{name: "managed hidden bypass", runner: unusedRunner(), config: Config{ExtraArgs: []string{"--dangerously-skip-permissions"}}, field: "extra_args[0]"},
		{name: "interactive mode", runner: unusedRunner(), config: Config{ExtraArgs: []string{"--interactive"}}, field: "extra_args[0]"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewImplementer(test.runner, test.config, nil)
			var configurationErr *ConfigurationError
			if !errors.As(err, &configurationErr) {
				t.Fatalf("NewImplementer() error = %v; want ConfigurationError", err)
			}
			if configurationErr.Field != test.field {
				t.Fatalf("error field = %q; want %q", configurationErr.Field, test.field)
			}
		})
	}
}

func unusedRunner() *fakeProcessRunner {
	return &fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		return process.Result{}, errors.New("unused")
	}}
}
