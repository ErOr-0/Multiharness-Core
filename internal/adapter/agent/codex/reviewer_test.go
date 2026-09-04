package codex

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
)

func TestReviewerIncludesCompleteEvidenceAndRepositoryInspection(t *testing.T) {
	request := validReviewRequest(t)
	var captured invocationSnapshot
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		captured = captureInvocation(t, command)
		writeFinalResponse(t, command, `{
			"schema_version":"1",
			"approved":true,
			"summary":"Implementation satisfies the task and validation passed.",
			"findings":[],
			"suggestions":[]
		}`)
		return process.Result{ExitCode: 0}, nil
	}}
	reviewer, err := NewReviewer(runner, Config{})
	if err != nil {
		t.Fatalf("NewReviewer() returned an error: %v", err)
	}
	review, err := reviewer.Review(context.Background(), request)
	if err != nil {
		t.Fatalf("Review() returned an error: %v", err)
	}
	if !review.Approved {
		t.Fatal("Review().Approved = false; want true")
	}

	encoded, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() returned an error: %v", err)
	}
	for _, expected := range []string{
		string(encoded),
		"live repository state and diff",
		"staged, unstaged, and untracked files",
		"complete tracked diff against HEAD",
		"claims to verify",
		"deterministic validation evidence",
	} {
		if !strings.Contains(captured.prompt, expected) {
			t.Errorf("review prompt does not contain %q", expected)
		}
	}
	if !strings.Contains(string(captured.schema), `"schema_version"`) ||
		!strings.Contains(string(captured.schema), `"enum": ["1"]`) {
		t.Fatal("command did not receive the versioned review schema")
	}
	if argumentValue(t, captured.args, "--sandbox") != string(SandboxReadOnly) {
		t.Fatalf("review sandbox args = %#v", captured.args)
	}
}

func TestReviewerStartsIndependentEphemeralInvocations(t *testing.T) {
	request := validReviewRequest(t)
	var invocations []invocationSnapshot
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		invocations = append(invocations, captureInvocation(t, command))
		writeFinalResponse(t, command, `{
			"schema_version":"1",
			"approved":true,
			"summary":"Approved.",
			"findings":[],
			"suggestions":[]
		}`)
		return process.Result{}, nil
	}}
	reviewer, err := NewReviewer(runner, Config{})
	if err != nil {
		t.Fatalf("NewReviewer() returned an error: %v", err)
	}
	for range 2 {
		if _, err := reviewer.Review(context.Background(), request); err != nil {
			t.Fatalf("Review() returned an error: %v", err)
		}
	}

	if len(invocations) != 2 {
		t.Fatalf("invocation count = %d; want 2", len(invocations))
	}
	for _, invocation := range invocations {
		if !contains(invocation.args, "--ephemeral") {
			t.Errorf("args %#v do not contain --ephemeral", invocation.args)
		}
		for _, argument := range invocation.args {
			if argument == "resume" || argument == "--session" {
				t.Errorf("args %#v attempt to reuse a session", invocation.args)
			}
		}
	}
	if invocations[0].outputPath == invocations[1].outputPath {
		t.Fatalf("review invocations shared output path %q", invocations[0].outputPath)
	}
}
