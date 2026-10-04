package sessionexec

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/contract"
)

func TestImplementBuildsNonInteractiveCommandAndCapturesSession(t *testing.T) {
	request := validImplementationRequest(t)
	var captured invocationSnapshot
	runner := &fakeProcessRunner{run: func(
		_ context.Context,
		command process.Command,
	) (process.Result, error) {
		captured = captureInvocation(t, command)
		writeOutput(
			t,
			command,
			`{"type":"step_start","sessionID":"ses_new","part":{"type":"step-start"}}`+"\n"+
				`{"type":"tool_use","sessionID":"ses_new","part":{"type":"tool","tool":"edit","state":{"status":"completed"}}}`+"\n",
			`{"type":"text","sessionID":"ses_new","part":{"type":"text","text":"{\"schema_version\":\"1\",\"summary\":\"Implemented and tested the endpoint.\",\"changed_files\":[\"health.go\",\"health_test.go\"]}"}}`+"\n"+
				`{"type":"step_finish","sessionID":"ses_new","part":{"type":"step-finish","reason":"stop"}}`+"\n",
		)
		return process.Result{ExitCode: 0}, nil
	}}
	implementer, err := NewImplementer(runner, Config{})
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
	for _, expected := range []string{
		`"task":"Add a health endpoint"`,
		`"summary":"Add and verify the endpoint."`,
		`"workspace_fingerprint"`,
		`"pre_existing_file_count"`,
		"preserve unrelated existing changes",
		`"schema_version":"1"`,
	} {
		if !strings.Contains(captured.prompt, expected) {
			t.Fatalf("implementation prompt is missing %q: %q", expected, captured.prompt)
		}
	}
	for _, absent := range []string{`"pre_existing_files"`, `"diff":`} {
		if strings.Contains(captured.prompt, absent) {
			t.Fatalf("implementation prompt carries unbounded evidence %q", absent)
		}
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
	implementer, err := NewImplementer(runner, config)
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
		if !slices.Contains(captured.args, expected) {
			t.Errorf("command args %#v do not contain %q", captured.args, expected)
		}
	}
}

func TestApplyReviewRejectsInvalidPriorSessionBeforeExecution(t *testing.T) {
	request := validRepairRequest(t)
	request.Implementation.AgentSessionID = " ses_invalid "
	runner := &fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("runner called with invalid prior session")
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{})
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
	implementer, err := NewImplementer(runner, Config{})
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	_, err = implementer.Implement(context.Background(), contract.ImplementationRequest{})
	if err == nil || runner.calls != 0 {
		t.Fatalf("Implement() error/calls = %v/%d; want validation error and zero calls", err, runner.calls)
	}
}

func TestApplyReviewRejectsInvalidRequestBeforeExecution(t *testing.T) {
	runner := &fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("runner called for invalid input")
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{})
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	_, err = implementer.ApplyReview(context.Background(), contract.RepairRequest{})
	if err == nil || runner.calls != 0 {
		t.Fatalf("ApplyReview() error/calls = %v/%d; want validation error and zero calls", err, runner.calls)
	}
}

func TestImplementRejectsNilContext(t *testing.T) {
	runner := &fakeProcessRunner{run: func(context.Context, process.Command) (process.Result, error) {
		t.Fatal("runner called with nil context")
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{})
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	//lint:ignore SA1012 a nil context must be rejected, not dereferenced
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
	implementer, err := NewImplementer(runner, Config{})
	if err != nil {
		t.Fatalf("NewImplementer() returned an error: %v", err)
	}

	//lint:ignore SA1012 a nil context must be rejected, not dereferenced
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
		writeOutput(
			t,
			command,
			`{"type":"step_start","sessionID":"ses_prior_123","part":{"type":"step-start"}}`+"\n"+
				`{"type":"text","sessionID":"ses_prior_123","part":{"type":"text","text":"{\"schema_version\":\"1\",\"summary\":\"Resumed and fixed\",\"changed_files\":[]}"}}`+"\n"+
				`{"type":"step_finish","sessionID":"ses_prior_123","part":{"type":"step-finish","reason":"stop"}}`+"\n",
		)
		return process.Result{ExitCode: 0}, nil
	}}
	implementer, err := NewImplementer(runner, Config{})
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
