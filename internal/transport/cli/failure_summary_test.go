package cli

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type failureOutputRunner struct{ payload []byte }

func (r failureOutputRunner) Run(_ context.Context, cmd process.Command) (process.Result, error) {
	_, err := cmd.Stdout.Write(r.payload)
	return process.Result{}, err
}

func TestFailureSummaryFromProviderThroughProgressKeepsCodeSeparate(t *testing.T) {
	code := strings.Repeat("    _logger.LogError(ex, \"source code, not a diagnostic\");\n", 500)
	data, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{
		"type": "command_execution", "status": "failed", "command": "cat source.cs; rg missing-file", "exit_code": 2,
		"aggregated_output": code + "rg: missing-file: No such file or directory\nFINAL OUTPUT",
	}})
	sink := newPresentation(io.Discard, io.Discard).progress
	sink.Publish(workflow.Event{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStagePlanning})
	runner := activity.Runner{Agent: activity.Codex, Runner: failureOutputRunner{append(data, '\n')}, Observe: sink.AgentActivity}
	_, err := runner.Run(t.Context(), process.Command{Args: []string{"exec", "--json"}})
	if err != nil {
		t.Fatal(err)
	}
	sink.Publish(workflow.Event{Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageImplementation})
	events, count := sink.failureDetails()
	if count != 1 || len(events) != 1 || events[0].Stage != "planning" {
		t.Fatal(events, count)
	}
	p := newFailurePager(events, count)
	v := &interactiveView{}
	summary := p.render(v, 100, 30)
	for _, want := range []string{"Tool failure details · planning", "cat source.cs; rg missing-file", "No such file or directory", "OUTPUT DIAGNOSTICS"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("missing %q: %s", want, summary)
		}
	}
	if strings.Contains(summary, "_logger.LogError") || strings.Contains(summary, "FINAL OUTPUT") {
		t.Fatal("source output mixed into summary")
	}
	if narrow := p.render(v, 40, 20); !strings.Contains(narrow, "command exited 2") || !strings.Contains(narrow, "planning") || !strings.Contains(narrow, "rg: missing-file") || strings.Contains(narrow, "_logger.LogError") {
		t.Fatal("narrow summary hid status, stage or diagnostic", narrow)
	}
	p.input("\x1b[<0;3;4M")
	if frame := p.render(v, 100, 30); !strings.Contains(frame, "_logger.LogError") {
		t.Fatal("output arrow did not expand output", frame)
	}
	p.input("\x1b[F")
	if frame := p.render(v, 100, 30); !strings.Contains(frame, "FINAL OUTPUT") || strings.Contains(frame, "truncated at 8 KiB") {
		t.Fatal("final output was lost", frame)
	}
	p.input("o")
	if frame := p.render(v, 100, 30); strings.Contains(frame, "_logger.LogError") || !strings.Contains(frame, "OUTPUT DIAGNOSTICS") {
		t.Fatal("output did not collapse back to summary", frame)
	}
}

func TestFailureOverviewDoesNotInventAnErrorFromSource(t *testing.T) {
	p := newFailurePager([]activity.Event{{Detailed: true, Command: "python - <<'PY'\nprint('embedded source')\nPY", Output: "_logger.LogError(ex);\ncommandSucceeded = false;"}}, 1)
	frame := p.render(&interactiveView{}, 80, 24)
	if strings.Contains(frame, "embedded source") || strings.Contains(frame, "_logger.LogError") || strings.Contains(frame, "OUTPUT DIAGNOSTICS") || !strings.Contains(frame, "does not identify the cause") {
		t.Fatal(frame)
	}
	p.input("\x1b[<0;17;4M")
	if frame = p.render(&interactiveView{}, 80, 24); !strings.Contains(frame, "embedded source") {
		t.Fatal("complete command unavailable", frame)
	}
	p.input("c")
	if frame = p.render(&interactiveView{}, 80, 24); strings.Contains(frame, "embedded source") {
		t.Fatal("command did not collapse", frame)
	}
}
