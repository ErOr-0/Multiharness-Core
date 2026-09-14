package cli_test

import (
	"bytes"
	"context"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
	"path/filepath"
	"strings"
	"testing"
)

func TestTeamPermissionBlockRendersActionableWait(t *testing.T) {
	var out bytes.Buffer
	h := newTeamHandler(t, func(_ config.Config, sink workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(context.Context, store.TaskInput) store.TaskOutput {
			sink.Publish(workflow.Event{Type: workflow.EventTypeStageFailed, Stage: store.WorkflowStageImplementation, Status: store.TaskStatusNeedsInput, FailureCode: store.FailureCodePermission})
			return store.TaskOutput{Status: store.TaskStatusNeedsInput, Summary: "Permission needed", AgentInvocations: 2, Failure: &store.TaskFailure{Stage: store.WorkflowStageImplementation, Code: store.FailureCodePermission, Message: "Read denied", Permission: &store.PermissionDenied{SessionID: "ses_denied", Action: store.BlockedAction{Tool: "read", Target: "/cache/module.go"}}}}
		}), nil
	}, &out, &out, t.TempDir(), nil)
	if code := h.Interactive(t.Context(), &promptLines{lines: []string{"task", "/quit"}}, filepath.Join(t.TempDir(), "config.json")); code != 0 {
		t.Fatal(code, out.String())
	}
	for _, expected := range []string{"status=needs_input", "exit_code=4", "permission_denied", "/permissions", "resubmit your original task"} {
		if !strings.Contains(out.String(), expected) {
			t.Fatalf("missing %q: %s", expected, out.String())
		}
	}
	if strings.Contains(out.String(), "[redacted]") || strings.Contains(out.String(), "[FAIL]") {
		t.Fatal(out.String())
	}
}
