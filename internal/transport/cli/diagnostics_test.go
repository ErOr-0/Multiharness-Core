package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func TestInteractiveRetainsSafeProviderFailureAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, "config.json")
	cfg := config.Defaults()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	calls := 0
	factory := func(config.Config, workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(context.Context, contract.TaskInput) contract.TaskOutput {
			calls++
			return contract.TaskOutput{Status: contract.TaskStatusFailed, Summary: "private-task-text", AgentInvocations: 1,
				Failure: &contract.TaskFailure{Stage: contract.WorkflowStagePlanning, Code: contract.FailureCodeAgent, Message: "private-provider-text",
					Provider: &contract.ProviderFailure{Kind: contract.ProviderConnection, Reason: "stream_disconnected", Source: "turn.failed", Attempts: 1}}}
		}), nil
	}
	h := newHandler(t, factory, &out, &stderr, dir, nil)
	if code := h.Interactive(t.Context(), &promptLines{lines: []string{"private-task-text", "/quit"}}, settings); code != 0 {
		t.Fatalf("exit=%d %s", code, out.String())
	}
	filename := filepath.Join(dir, "last-provider-failure.json")
	saved, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "private-") || !strings.Contains(string(saved), "stream_disconnected") || !strings.Contains(string(saved), "run_") {
		t.Fatalf("unsafe or incomplete diagnostic: %s", saved)
	}
	out.Reset()
	if code := h.Interactive(t.Context(), &promptLines{lines: []string{"/diagnostics", "/quit"}}, settings); code != 0 {
		t.Fatal(code)
	}
	if calls != 1 || !strings.Contains(out.String(), "stream_disconnected") || strings.Contains(out.String(), "private-") {
		t.Fatalf("diagnostic retrieval failed: %s", out.String())
	}
	// Corrupt diagnostic fields must not introduce terminal controls or secrets.
	if err := os.WriteFile(filename, bytes.ReplaceAll(saved, []byte("stream_disconnected"), []byte("secret-injected-reason")), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	_ = h.Interactive(t.Context(), &promptLines{lines: []string{"/diagnostics", "/quit"}}, settings)
	if strings.Contains(out.String(), "secret-injected") || !strings.Contains(out.String(), "diagnostics are invalid") {
		t.Fatal("untrusted diagnostic was rendered")
	}
}
