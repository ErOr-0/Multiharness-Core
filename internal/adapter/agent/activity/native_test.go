package activity

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNativeCodexFailureDetails(t *testing.T) {
	for _, tc := range []struct {
		name, item, summary, command, output, failure string
	}{
		{"command", `{"type":"commandExecution","status":"failed","command":"cat missing --password=hunter2","aggregatedOutput":"cat: missing: No such file\npassword=hunter2","exitCode":1}`, "command exited 1", "cat missing --password=[redacted]", "cat: missing: No such file\npassword=[redacted]", ""},
		{"nonzero completed", `{"type":"commandExecution","status":"completed","command":"grep needle file","aggregatedOutput":"","exitCode":1}`, "command exited 1", "grep needle file", "", ""},
		{"mcp", `{"type":"mcpToolCall","status":"failed","arguments":{"secret":"private args"},"error":{"message":"server rejected api_key=hunter2"}}`, "MCP tool failed", "", "", "server rejected api_key=[redacted]"},
		{"patch", `{"type":"fileChange","status":"failed","changes":[{"diff":"private diff"}],"error":{"message":"patch rejected"}}`, "file change failed", "", "", "patch rejected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []Event
			o := observer{agent: Codex, publish: func(e Event) { events = append(events, e) }}
			// Exercise fragmented native JSONL at the same boundary as process output.
			wire := `{"method":"item/completed","params":{"item":` + tc.item + "}}\n"
			for _, b := range []byte(wire) {
				_, _ = o.Write([]byte{b})
			}
			if len(events) != 1 {
				t.Fatalf("events: %+v", events)
			}
			e := events[0]
			if e.Kind != ToolFailed || !e.Detailed || e.Summary != tc.summary || e.Command != tc.command || e.Output != tc.output || e.Error != tc.failure {
				t.Fatalf("failure detail: %+v", e)
			}
			metadata, _ := json.Marshal(e)
			if strings.Contains(string(metadata), "rejected") || strings.Contains(e.Text, "private") || strings.Contains(e.Text, "hunter2") {
				t.Fatalf("private fields leaked: %s %+v", metadata, e)
			}
		})
	}
}

func TestNativeCodexCommandSuccessAndFullFailureOutput(t *testing.T) {
	e, ok := nativeActivity(Codex, []byte(`{"method":"item/completed","params":{"item":{"type":"commandExecution","status":"completed","exitCode":0}}}`))
	if !ok || e.Kind != CommandFinished || e.Detailed {
		t.Fatalf("successful command: %+v", e)
	}
	output := strings.Repeat("build output\n", 2000) + "fatal: final diagnostic"
	wire, _ := json.Marshal(map[string]any{"method": "item/completed", "params": map[string]any{"item": map[string]any{"type": "commandExecution", "status": "failed", "aggregatedOutput": output}}})
	e, ok = nativeActivity(Codex, wire)
	if !ok || e.Output != output || len(e.Text) >= len(output) {
		t.Fatal("full diagnostic was lost or preview was not bounded")
	}
}

func TestNativeProgressDoesNotExposeApprovalOrReasoning(t *testing.T) {
	for _, data := range []string{`{"method":"item/fileChange/requestApproval","params":{"reason":"private approval"}}`, `{"method":"item/completed","params":{"item":{"type":"reasoning","text":"private reasoning"}}}`} {
		if event, ok := nativeActivity(Codex, []byte(data)); ok {
			t.Fatal(event)
		}
	}
	e, ok := nativeActivity(Codex, []byte(`{"method":"item/completed","params":{"item":{"type":"agentMessage","text":"Updated files"}}}`))
	if !ok || e.Kind != ResponseReceived || !strings.Contains(e.Text, "Updated files") {
		t.Fatal(e, ok)
	}
}
