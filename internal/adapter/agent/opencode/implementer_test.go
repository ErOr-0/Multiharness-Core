package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func TestImplementBuildsNonInteractiveCommandAndCapturesSession(t *testing.T) {
	request := validImplementationRequest(t)
	progress := &recordingProgressSink{}
	var captured invocationSnapshot
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		captured = captureInvocation(t, command)
		writeOutput(t, command,
			`{"type":"step_start","sessionID":"ses_new","part":{"type":"step-start"}}`+"\n"+
				`{"type":"tool_use","sessionID":"ses_new","part":{"type":"tool","tool":"edit","state":{"status":"completed"}}}`+"\n",
			`{"type":"text","sessionID":"ses_new","part":{"type":"text","text":"{\"schema_version\":\"1\",\"summary\":\"Implemented and tested the endpoint.\",\"changed_files\":[\"health.go\",\"health_test.go\"]}"}}`+"\n"+
				`{"type":"step_finish","sessionID":"ses_new","part":{"type":"step-finish"}}`+"\n",
		)
		return process.Result{ExitCode: 0}, nil
	}}
	implementer, err := NewImplementer(runner, Config{}, progress)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	result, err := implementer.Implement(context.Background(), request)
	if err != nil {
		t.Fatalf("Implement() returned an error: %v", err)
	}
	if result.AgentSessionID != "ses_new" || result.Summary != "Implemented and tested the endpoint." {
		t.Fatalf("implementation result = %#v", result)
	}
	if !reflect.DeepEqual(result.ChangedFiles, []string{"health.go", "health_test.go"}) {
		t.Fatalf("changed files = %#v", result.ChangedFiles)
	}

	expectedArguments := []string{"run", "--format", "json", "--dir", request.Input.WorkingDir}
	if captured.name != DefaultExecutable || !reflect.DeepEqual(captured.args, expectedArguments) {
		t.Fatalf("command name/args = %q/%#v; want %q/%#v", captured.name, captured.args, DefaultExecutable, expectedArguments)
	}
	if captured.dir != request.Input.WorkingDir || time.Duration(captured.timeout) != DefaultTimeout {
		t.Fatalf("command dir/timeout = %q/%s", captured.dir, time.Duration(captured.timeout))
	}
	if strings.Contains(strings.Join(captured.args, " "), request.Input.Task) {
		t.Fatal("task prompt leaked into process arguments")
	}
	encodedRequest, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent() returned an error: %v", err)
	}
	if !strings.Contains(captured.prompt, string(encodedRequest)) ||
		!strings.Contains(captured.prompt, "preserve unrelated existing changes") ||
		!strings.Contains(captured.prompt, `"schema_version":"1"`) {
		t.Fatalf("implementation prompt = %q", captured.prompt)
	}

	expectedProgress := []ProgressEvent{
		{Type: ProgressStepStarted, SessionID: "ses_new"},
		{Type: ProgressToolFinished, SessionID: "ses_new", Tool: "edit", Status: "completed"},
		{Type: ProgressStepFinished, SessionID: "ses_new"},
	}
	if !reflect.DeepEqual(progress.events, expectedProgress) {
		t.Fatalf("progress events = %#v; want %#v", progress.events, expectedProgress)
	}
}

func TestImplementUsesConfigurationOverridesAndAutoApproval(t *testing.T) {
	request := validImplementationRequest(t)
	var captured invocationSnapshot
	config := Config{
		Executable:       "/opt/bin/opencode-custom",
		Model:            "openai/gpt-custom",
		Variant:          "max",
		Timeout:          75 * time.Minute,
		PermissionPolicy: PermissionAutoApprove,
		ExtraArgs:        []string{"--pure", "--log-level=WARN"},
	}
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		captured = captureInvocation(t, command)
		writeOutput(t, command, successfulEventStream("ses_custom", "Done.", "main.go"))
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, config, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	if _, err := implementer.Implement(context.Background(), request); err != nil {
		t.Fatalf("Implement() returned an error: %v", err)
	}
	if captured.name != config.Executable || time.Duration(captured.timeout) != config.Timeout {
		t.Fatalf("command name/timeout = %q/%s", captured.name, time.Duration(captured.timeout))
	}
	for _, expected := range []string{
		"--model", config.Model,
		"--variant", config.Variant,
		"--auto", "--pure", "--log-level=WARN",
	} {
		if !contains(captured.args, expected) {
			t.Errorf("command args %#v do not contain %q", captured.args, expected)
		}
	}
}

func TestApplyReviewResumesSessionAndSuppliesCompleteRepairEvidence(t *testing.T) {
	request := validRepairRequest(t)
	var captured invocationSnapshot
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		captured = captureInvocation(t, command)
		writeOutput(t, command, successfulEventStream("ses_original", "Fixed the status.", "health.go"))
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	result, err := implementer.ApplyReview(context.Background(), request)
	if err != nil {
		t.Fatalf("ApplyReview() returned an error: %v", err)
	}
	if result.AgentSessionID != request.Implementation.AgentSessionID {
		t.Fatalf("repair session = %q; want %q", result.AgentSessionID, request.Implementation.AgentSessionID)
	}
	if actual := argumentValue(t, captured.args, "--session"); actual != request.Implementation.AgentSessionID {
		t.Fatalf("--session = %q; want %q", actual, request.Implementation.AgentSessionID)
	}
	for _, evidence := range []string{
		request.Input.Task,
		request.Plan.Summary,
		request.Validation.Checks[0].Output,
		request.Review.Findings[0].Evidence,
		request.Review.Findings[0].RequiredAction,
	} {
		if !strings.Contains(captured.prompt, evidence) {
			t.Errorf("repair prompt does not contain %q", evidence)
		}
	}
	if strings.Contains(captured.prompt, request.Review.Findings[1].Description) ||
		strings.Contains(captured.prompt, request.Review.Suggestions[0]) {
		t.Fatal("repair prompt included non-blocking suggestions")
	}
}

func TestApplyReviewStartsFreshSessionWhenPriorSessionIsUnavailable(t *testing.T) {
	request := validRepairRequest(t)
	request.Implementation.AgentSessionID = ""
	var arguments []string
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		arguments = slices.Clone(command.Args)
		writeOutput(t, command, successfulEventStream("ses_recovery", "Fixed.", "health.go"))
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	result, err := implementer.ApplyReview(context.Background(), request)
	if err != nil {
		t.Fatalf("ApplyReview() returned an error: %v", err)
	}
	if contains(arguments, "--session") || result.AgentSessionID != "ses_recovery" {
		t.Fatalf("fresh repair args/result = %#v/%#v", arguments, result)
	}
}

func TestApplyReviewRejectsInvalidPriorSessionBeforeExecution(t *testing.T) {
	request := validRepairRequest(t)
	request.Implementation.AgentSessionID = " ses_invalid "
	runner := &fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("runner called with invalid prior session")
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	_, err = implementer.ApplyReview(context.Background(), request)
	var outputErr *OutputError
	if !errors.As(err, &outputErr) || outputErr.SessionID != request.Implementation.AgentSessionID {
		t.Fatalf("ApplyReview() error = %v; want session-aware OutputError", err)
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d; want 0", runner.calls)
	}
}

func TestImplementRejectsInvalidRequestBeforeExecution(t *testing.T) {
	runner := &fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("runner called for invalid input")
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	_, err = implementer.Implement(context.Background(), store.ImplementationRequest{})
	if err == nil || runner.calls != 0 {
		t.Fatalf("Implement() error/calls = %v/%d; want validation error and zero calls", err, runner.calls)
	}
}

func TestApplyReviewRejectsInvalidRequestBeforeExecution(t *testing.T) {
	runner := &fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("runner called for invalid input")
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	_, err = implementer.ApplyReview(context.Background(), store.RepairRequest{})
	if err == nil || runner.calls != 0 {
		t.Fatalf("ApplyReview() error/calls = %v/%d; want validation error and zero calls", err, runner.calls)
	}
}

func TestImplementRejectsNilContext(t *testing.T) {
	runner := &fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("runner called with nil context")
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	_, err = implementer.Implement(nil, validImplementationRequest(t))
	if !errors.Is(err, errNilContext) || runner.calls != 0 {
		t.Fatalf("Implement() error/calls = %v/%d; want nil-context error and zero calls", err, runner.calls)
	}
}

func TestApplyReviewRejectsNilContext(t *testing.T) {
	runner := &fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("runner called with nil context")
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	_, err = implementer.ApplyReview(nil, validRepairRequest(t))
	if !errors.Is(err, errNilContext) || runner.calls != 0 {
		t.Fatalf("ApplyReview() error/calls = %v/%d; want nil-context error and zero calls", err, runner.calls)
	}
}

func TestImplementResumesSessionWhenProvidedInInput(t *testing.T) {
	request := validImplementationRequest(t)
	request.Input.SessionID = "ses_prior_123"
	var capturedArgs []string
	runner := &fakeProcessRunner{run: func(_ context.Context, command process.Command) (process.Result, error) {
		capturedArgs = append([]string{}, command.Args...)
		writeOutput(t, command,
			`{"type":"step_start","sessionID":"ses_prior_123","part":{"type":"step-start"}}`+"\n"+
				`{"type":"text","sessionID":"ses_prior_123","part":{"type":"text","text":"{\"schema_version\":\"1\",\"summary\":\"Resumed and fixed\",\"changed_files\":[]}"}}`+"\n"+
				`{"type":"step_finish","sessionID":"ses_prior_123","part":{"type":"step-finish"}}`+"\n",
		)
		return process.Result{ExitCode: 0}, nil
	}}
	implementer, err := NewImplementer(runner, Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := implementer.Implement(context.Background(), request)
	if err != nil {
		t.Fatalf("Implement() returned error: %v", err)
	}
	if result.AgentSessionID != "ses_prior_123" {
		t.Fatalf("result.AgentSessionID = %q; want ses_prior_123", result.AgentSessionID)
	}
	hasSession := false
	for i, arg := range capturedArgs {
		if arg == "--session" && i+1 < len(capturedArgs) && capturedArgs[i+1] == "ses_prior_123" {
			hasSession = true
			break
		}
	}
	if !hasSession {
		t.Fatalf("captured args %#v did not contain --session ses_prior_123", capturedArgs)
	}
}
