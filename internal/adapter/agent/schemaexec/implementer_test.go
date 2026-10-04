package schemaexec

import (
	"context"
	"slices"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/contract"
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
	result, err := impl.Implement(t.Context(), contract.ImplementationRequest{Input: request.Input, Plan: request.Plan})
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
