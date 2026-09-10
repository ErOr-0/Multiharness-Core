package structured_test

import (
	"context"
	"errors"
	"testing"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
)

// A protocol needs no provider-specific role methods to use the workflow contract.
func TestSharedAgentRejectsInvalidResultsAndPreservesFailures(t *testing.T) {
	input := store.TaskInput{Task: "explain this project", WorkingDir: t.TempDir()}
	failure := errors.New("protocol failed")
	for _, tc := range []struct {
		name, data string
		failure    error
	}{
		{name: "invalid result", data: `{}`},
		{name: "cancelled", failure: context.Canceled},
		{name: "provider failure", failure: failure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agent := structured.Agent{Execute: func(ctx context.Context, r structured.Invocation) (structured.Response, error) {
				if ctx != t.Context() || r.Role != "planning" || r.WorkingDir != input.WorkingDir {
					t.Fatal("lost invocation context")
				}
				return structured.Response{Data: []byte(tc.data)}, tc.failure
			}}
			_, err := agent.Plan(t.Context(), input)
			if err == nil || (tc.failure != nil && !errors.Is(err, tc.failure)) {
				t.Fatalf("lost failure: %v", err)
			}
		})
	}
}

func TestSharedAgentEnforcesReadOnlyRoleBoundary(t *testing.T) {
	agent := structured.Agent{CanWrite: true, Execute: func(context.Context, structured.Invocation) (structured.Response, error) {
		t.Fatal("write-capable protocol invoked for read-only role")
		return structured.Response{}, nil
	}}
	if _, err := agent.Plan(t.Context(), store.TaskInput{Task: "explain", WorkingDir: t.TempDir()}); err == nil {
		t.Fatal("accepted writable planner")
	}
	if _, err := agent.Review(t.Context(), store.ReviewRequest{}); err == nil {
		t.Fatal("accepted writable reviewer")
	}
}
