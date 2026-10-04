package console_test

import (
	"bytes"
	"os"
	"testing"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/console"
)

func TestPipedYesIsNeverConsent(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	_, _ = writer.WriteString("yes\n")
	writer.Close()
	var output bytes.Buffer
	approver := console.NewApprover(reader, &output)
	if approver == nil {
		return
	}
	yes, err := approver.ConfirmFallback(t.Context(), contract.AgentSwitch{Stage: contract.WorkflowStageImplementation, From: "OpenCode", To: "Codex", Model: "test-model", CanWrite: true})
	if err != nil || yes || output.Len() != 0 {
		t.Fatal("non-interactive input authorized fallback")
	}
}
