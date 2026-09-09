package activity

import (
	"strings"
	"testing"
)

func TestVisibleOutputArrivesBeforeProcessExit(t *testing.T) {
	var events []Event
	o := observer{agent: Codex, publish: func(e Event) { events = append(events, e) }}
	lines := []string{
		`{"type":"item.completed","item":{"type":"agent_message","text":"Checking changed files"}}`,
		`{"type":"item.started","item":{"type":"command_execution","command":"git diff --stat"}}`,
		`{"type":"item.completed","item":{"type":"command_execution","command":"git diff --stat","aggregated_output":"2 files changed","exit_code":0}}`,
	}
	for _, line := range lines {
		_, _ = o.Write([]byte(line + "\n"))
	}
	if len(events) != 3 || events[0].Text != "Checking changed files" || events[1].Text != "$ git diff --stat" || !strings.Contains(events[2].Text, "[shell exit 0; not a validation result]") {
		t.Fatalf("missing streamed output: %+v", events)
	}
	_, _ = o.Write([]byte(`{"type":"item.completed","item":{"type":"reasoning","text":"private reasoning"}}` + "\n"))
	if len(events) != 3 {
		t.Fatal("reasoning exposed")
	}
}

func TestDisplayFiltersCredentialsControlsAndBounds(t *testing.T) {
	raw := "\x1b[2JAuthorization: Bearer very-private\napi_key='hidden-value'\npassword=hunter2\n" + strings.Repeat("x", 10000)
	got := DisplayText(raw)
	for _, secret := range []string{"\x1b", "very-private", "hidden-value", "hunter2"} {
		if strings.Contains(got, secret) {
			t.Fatalf("unsafe output %q", secret)
		}
	}
	if !strings.Contains(got, "[output truncated") || len(got) > 8300 {
		t.Fatal("unbounded output")
	}
}

func TestVisibleProviderError(t *testing.T) {
	data := []byte(`{"type":"turn.failed","error":{"message":"connection closed password=hidden"}}`)
	if decode(Codex, data) != ToolFailed || visibleText(Codex, data) != "connection closed password=[redacted]" {
		t.Fatal("provider error message missing or unfiltered")
	}
}

func TestVisibleNullErrorAndLongCommandExit(t *testing.T) {
	got := visibleText(Codex, []byte(`{"type":"error","error":null,"message":"connection unavailable"}`))
	if got != "connection unavailable" {
		t.Fatal("top-level error hidden by null payload")
	}
	data := []byte(`{"type":"item.completed","item":{"type":"command_execution","command":"build","exit_code":1,"aggregated_output":"` + strings.Repeat("x", 20000) + `"}}`)
	got = visibleText(Codex, data)
	if !strings.Contains(got, "shell exit 1") || !strings.Contains(got, "output truncated") {
		t.Fatal("long output hid command status")
	}
}
