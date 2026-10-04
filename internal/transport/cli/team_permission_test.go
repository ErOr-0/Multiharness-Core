package cli_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func TestTeamPermissionBlockRendersActionableWait(t *testing.T) {
	var out bytes.Buffer
	h := newTeamHandler(t, func(_ config.Config, sink workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(context.Context, contract.TaskInput) contract.TaskOutput {
			sink.Publish(workflow.Event{Type: workflow.EventTypeStageFailed, Stage: contract.WorkflowStageImplementation, Status: contract.TaskStatusNeedsInput, FailureCode: contract.FailureCodePermission})
			return contract.TaskOutput{Status: contract.TaskStatusNeedsInput, Summary: "Permission needed", AgentInvocations: 2, Failure: &contract.TaskFailure{Stage: contract.WorkflowStageImplementation, Code: contract.FailureCodePermission, Message: "Read denied", Permission: &contract.PermissionDenied{SessionID: "ses_denied", Action: contract.BlockedAction{Tool: "read", Target: "/cache/module.go"}}}}
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
