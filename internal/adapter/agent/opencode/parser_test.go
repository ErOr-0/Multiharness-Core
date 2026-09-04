package opencode

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"multiharness-core/internal/store"
)

func TestEventStreamParsesChunksAndSurfacesStableProgress(t *testing.T) {
	sink := &recordingProgressSink{}
	stream := newEventStream("", sink)
	chunks := []string{
		"\r\n" + `{"type":"future_event","sessionID":"ses_123","extra":true}` + "\r\n" +
			`{"type":"step_start","sessionID":"ses_123","part":{"type":"step-start"}}` + "\n" +
			`{"type":"tool_use","sessionID":"ses_123","part":{"type":"tool","tool":"bash","state":{"status":"error"}}}`,
		"\n" + `{"type":"text","sessionID":"ses_123","part":{"type":"text","text":"working"}}` + "\n" +
			`{"type":"text","sessionID":"ses_123","part":{"type":"text","text":"{\"schema_version\":\"1\",\"summary\":\"done\",\"changed_files\":[]}"}}` + "\n" +
			`{"type":"step_finish","sessionID":"ses_123","part":{"type":"step-finish"}}`,
	}
	for _, chunk := range chunks {
		if _, err := stream.Write([]byte(chunk)); err != nil {
			t.Fatalf("Write() returned an error: %v", err)
		}
	}

	result, err := stream.finish()
	if err != nil {
		t.Fatalf("finish() returned an error: %v", err)
	}
	if result.sessionID != "ses_123" || !strings.Contains(result.finalText, `"summary":"done"`) {
		t.Fatalf("parsed events = %#v", result)
	}
	expectedProgress := []ProgressEvent{
		{Type: ProgressStepStarted, SessionID: "ses_123"},
		{Type: ProgressToolFinished, SessionID: "ses_123", Tool: "bash", Status: "error"},
		{Type: ProgressStepFinished, SessionID: "ses_123"},
	}
	if !reflect.DeepEqual(sink.events, expectedProgress) {
		t.Fatalf("progress events = %#v; want %#v", sink.events, expectedProgress)
	}
}

func TestEventStreamRejectsMalformedOrInconsistentEvents(t *testing.T) {
	oversized := strings.Repeat("x", maximumEventBytes+1)
	tests := []struct {
		name     string
		expected string
		output   string
		want     string
	}{
		{name: "blank", output: " \r\n", want: "empty"},
		{name: "malformed JSON", output: "{bad}\n", want: "decode JSON event"},
		{name: "missing type", output: `{"sessionID":"ses_1"}` + "\n", want: "type is missing"},
		{name: "missing session", output: `{"type":"text","part":{"type":"text","text":"done"}}` + "\n", want: "sessionID is missing"},
		{name: "invalid session", output: `{"type":"future","sessionID":" ses_1 "}` + "\n", want: "sessionID is invalid"},
		{
			name:   "session changed",
			output: `{"type":"future","sessionID":"ses_1"}` + "\n" + `{"type":"future","sessionID":"ses_2"}` + "\n",
			want:   "sessionID changed",
		},
		{
			name:     "resumed session mismatch",
			expected: "ses_expected",
			output:   `{"type":"future","sessionID":"ses_other"}` + "\n",
			want:     "does not match resumed session",
		},
		{name: "missing part", output: `{"type":"text","sessionID":"ses_1"}` + "\n", want: "part is missing"},
		{
			name:   "wrong part type",
			output: `{"type":"step_start","sessionID":"ses_1","part":{"type":"step-finish"}}` + "\n",
			want:   "does not match",
		},
		{
			name:   "blank tool",
			output: `{"type":"tool_use","sessionID":"ses_1","part":{"type":"tool","state":{"status":"completed"}}}` + "\n",
			want:   "tool is missing",
		},
		{
			name:   "unsupported tool status",
			output: `{"type":"tool_use","sessionID":"ses_1","part":{"type":"tool","tool":"edit","state":{"status":"running"}}}` + "\n",
			want:   "unsupported tool status",
		},
		{name: "oversized line", output: oversized, want: "exceeds"},
		{
			name:   "missing final text",
			output: `{"type":"step_start","sessionID":"ses_1","part":{"type":"step-start"}}` + "\n",
			want:   "final text",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream := newEventStream(test.expected, nil)
			if _, err := stream.Write([]byte(test.output)); err != nil {
				t.Fatalf("Write() returned an error: %v", err)
			}
			_, err := stream.finish()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("finish() error = %v; want containing %q", err, test.want)
			}
		})
	}
}

func TestEventStreamCapturesAgentErrorsWithoutRequiringText(t *testing.T) {
	stream := newEventStream("", nil)
	_, _ = stream.Write([]byte(
		`{"type":"error","sessionID":"ses_error","error":{"name":"ProviderError","data":{"message":"authentication failed"}}}`,
	))

	result, err := stream.finish()
	if err != nil {
		t.Fatalf("finish() returned an error: %v", err)
	}
	if result.agentError != "authentication failed" || result.sessionID != "ses_error" {
		t.Fatalf("parsed error event = %#v", result)
	}
}

func TestParseImplementationRejectsMalformedOrInvalidResponses(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		sessionID string
	}{
		{name: "empty", sessionID: "ses_1"},
		{name: "malformed", response: "{", sessionID: "ses_1"},
		{name: "fenced missing contract fields", response: "```json\n{}\n```", sessionID: "ses_1"},
		{name: "missing schema", response: `{"summary":"done","changed_files":[]}`, sessionID: "ses_1"},
		{name: "null summary", response: `{"schema_version":"1","summary":null,"changed_files":[]}`, sessionID: "ses_1"},
		{name: "missing changed files", response: `{"schema_version":"1","summary":"done"}`, sessionID: "ses_1"},
		{name: "unknown field", response: `{"schema_version":"1","summary":"done","changed_files":[],"extra":true}`, sessionID: "ses_1"},
		{name: "wrong schema", response: `{"schema_version":"2","summary":"done","changed_files":[]}`, sessionID: "ses_1"},
		{name: "blank summary", response: `{"schema_version":"1","summary":" ","changed_files":[]}`, sessionID: "ses_1"},
		{name: "blank changed file", response: `{"schema_version":"1","summary":"done","changed_files":[" "]}`, sessionID: "ses_1"},
		{name: "missing session", response: `{"schema_version":"1","summary":"done","changed_files":[]}`},
		{name: "multiple documents", response: `{"schema_version":"1","summary":"done","changed_files":[]} {}`, sessionID: "ses_1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseImplementation([]byte(test.response), test.sessionID)
			if err == nil {
				t.Fatal("parseImplementation() returned no error")
			}
		})
	}
}

func TestParseImplementationReturnsValidatedResult(t *testing.T) {
	result, err := parseImplementation([]byte(
		`{"schema_version":"1","summary":"Implemented the endpoint.","changed_files":["health.go"]}`,
	), "ses_1")
	if err != nil {
		t.Fatalf("parseImplementation() returned an error: %v", err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("result is invalid: %v", err)
	}
	if result.AgentSessionID != "ses_1" {
		t.Fatalf("agent session ID = %q; want ses_1", result.AgentSessionID)
	}
}

func TestAgentErrorCanBeInspected(t *testing.T) {
	err := &ExecutionError{Cause: &store.ProviderFailure{Kind: store.ProviderOverloaded, Attempts: 1}}
	var agentErr *store.ProviderFailure
	if !errors.As(err, &agentErr) {
		t.Fatalf("errors.As(%v) did not find ProviderFailure", err)
	}
}
