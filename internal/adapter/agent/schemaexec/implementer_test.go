package schemaexec

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func TestCodexImplementationIsWritableFreshAndSchemaConstrained(t *testing.T) {
	request := validReviewRequest(t)
	runner := &fakeProcessRunner{run: func(_ context.Context, c process.Command) (process.Result, error) {
		invocation := captureInvocation(t, c)
		if argumentValue(t, c.Args, "--sandbox") != "workspace-write" || !slices.Contains(c.Args, "--ephemeral") || !strings.Contains(string(invocation.schema), "changed_files") || !strings.Contains(invocation.prompt, "partial work") {
			t.Fatal("unsafe or incomplete implementation command")
		}
		writeFinalResponse(t, c, `{"schema_version":"1","summary":"implemented","changed_files":["health.go"]}`)
		return process.Result{}, nil
	}}
	cfg := DefaultConfig()
	cfg.Sandbox = SandboxWorkspaceWrite
	impl, err := NewImplementer(runner, cfg)
	if err != nil {
		t.Fatal(err)
	}
	result, err := impl.Implement(t.Context(), store.ImplementationRequest{Input: request.Input, Plan: request.Plan})
	if err != nil || result.AgentSessionID != "" {
		t.Fatalf("result=%+v error=%v", result, err)
	}
	if _, err := NewImplementer(runner, DefaultConfig()); err == nil {
		t.Fatal("accepted read-only implementation")
	}
	if _, err := NewImplementer(nil, cfg); err == nil {
		t.Fatal("accepted nil runner")
	}
}

func TestCodexRepairNeverImportsOpenCodeSession(t *testing.T) {
	r := validReviewRequest(t)
	r.Implementation.AgentSessionID = "opencode-session-secret"
	request := store.RepairRequest{
		Input:          r.Input,
		Plan:           r.Plan,
		Implementation: r.Implementation,
		Validation:     r.Validation,
		Review: store.Review{
			Summary:  "fix edge",
			Findings: []store.ReviewFinding{{Severity: store.FindingSeverityError, Blocking: true, Description: "broken", RequiredAction: "fix"}},
		},
	}
	runner := &fakeProcessRunner{run: func(_ context.Context, c process.Command) (process.Result, error) {
		invocation := captureInvocation(t, c)
		if strings.Contains(invocation.prompt, "opencode-session-secret") || !strings.Contains(invocation.prompt, "blocking_findings") {
			t.Fatal("incorrect handoff context")
		}
		writeFinalResponse(t, c, `{"schema_version":"1","summary":"fixed","changed_files":[]}`)
		return process.Result{}, nil
	}}
	cfg := DefaultConfig()
	cfg.Sandbox = SandboxWorkspaceWrite
	impl, _ := NewImplementer(runner, cfg)
	if _, err := impl.ApplyReview(t.Context(), request); err != nil {
		t.Fatal(err)
	}
	if _, err := impl.ApplyReview(nil, request); err == nil {
		t.Fatal("nil context accepted")
	}
	runner.run = func(context.Context, process.Command) (process.Result, error) {
		return process.Result{}, context.Canceled
	}
	if _, err := impl.ApplyReview(t.Context(), request); !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation")
	}
}
