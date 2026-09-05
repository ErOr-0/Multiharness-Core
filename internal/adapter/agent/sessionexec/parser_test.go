package sessionexec

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func TestEventStreamParsesChunksAndFinalResponse(t *testing.T) {
	stream := newEventStream("")
	chunks := []string{
		"\r\n" + `{"type":"future_event","sessionID":"ses_123","metadata":{"Type":"arbitrary","x-header":true,"example":"{\"type\":\"error\",\"type\":\"text\"}"}}` + "\r\n" +
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
		{
			name:   "missing session",
			output: `{"type":"text","part":{"type":"text","text":"done"}}` + "\n",
			want:   "sessionID is missing",
		},
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
		{
			name:   "tool status case alias",
			output: `{"type":"tool_use","sessionID":"ses_1","part":{"type":"tool","tool":"edit","state":{"status":"running","Status":"completed"}}}` + "\n",
			want:   "noncanonical JSON control field",
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
			stream := newEventStream(test.expected)
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
	stream := newEventStream("")
	_, _ = stream.Write([]byte(
		`{"type":"error","sessionID":"ses_error","error":{"name":"ProviderError","data":{"message":"authentication failed"}}}`,
	))

	result, err := stream.finish()
	if err != nil {
		t.Fatalf("finish() returned an error: %v", err)
	}
	if !result.agentFailed || result.sessionID != "ses_error" {
		t.Fatalf("parsed error event = %#v", result)
	}
}

func TestAmbiguousSessionEventsCannotCompleteImplementation(t *testing.T) {
	const valid = `{"type":"text","sessionID":"ses_1","part":{"type":"text","text":"{\"schema_version\":\"1\",\"summary\":\"done\",\"changed_files\":[]}"}}`
	for name, event := range map[string]string{
		"overwritten type":       strings.Replace(valid, `"type":"text"`, `"type":"error","type":"text"`, 1),
		"overwritten session":    strings.Replace(valid, `"sessionID":"ses_1"`, `"sessionID":"ses_other","sessionID":"ses_1"`, 1),
		"session case alias":     strings.Replace(valid, `"sessionID":"ses_1"`, `"sessionID":"ses_other","SESSIONID":"ses_1"`, 1),
		"overwritten part":       strings.Replace(valid, `"part":`, `"part":{"type":"text","text":"still working"},"part":`, 1),
		"overwritten final text": strings.Replace(valid, `"text":"{`, `"text":"still working","text":"{`, 1),
		"escaped final text":     strings.Replace(valid, `"text":"{`, `"text":"still working","te\u0078t":"{`, 1),
		"final text case alias":  strings.Replace(valid, `"text":"{`, `"text":"still working","TEXT":"{`, 1),
		"part type case alias":   strings.Replace(valid, `"part":{"type":"text"`, `"part":{"type":"error","TYPE":"text"`, 1),
		"invalid UTF-8":          strings.Replace(valid, "done", "done\xff", 1),
	} {
		t.Run(name, func(t *testing.T) {
			// The stream itself must reject ambiguity even when used without the
			// shared provider monitor, including keys inside the text part.
			stream := newEventStream("")
			_, _ = stream.Write([]byte(event))
			if _, err := stream.finish(); err == nil {
				t.Fatal("ambiguous event became a final response")
			}
			runner := &fakeProcessRunner{run: func(_ context.Context, command process.Command) (process.Result, error) {
				writeOutput(t, command, event+"\n")
				return process.Result{ExitCode: 0}, nil
			}}
			agent, err := NewImplementer(runner, Config{})
			if err != nil {
				t.Fatal(err)
			}
			result, err := agent.Implement(t.Context(), validImplementationRequest(t))
			if err == nil || result.Summary != "" || runner.calls != 1 {
				t.Fatalf("ambiguous event completed or replayed work: result=%+v err=%v calls=%d", result, err, runner.calls)
			}
			var failure *store.ProviderFailure
			if errors.As(err, &failure) && failure.Transient() {
				t.Fatal("ambiguous event must not become retryable")
			}
		})
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
		{
			name:      "unknown field",
			response:  `{"schema_version":"1","summary":"done","changed_files":[],"extra":true}`,
			sessionID: "ses_1",
		},
		{name: "wrong schema", response: `{"schema_version":"2","summary":"done","changed_files":[]}`, sessionID: "ses_1"},
		{name: "blank summary", response: `{"schema_version":"1","summary":" ","changed_files":[]}`, sessionID: "ses_1"},
		{
			name:      "blank changed file",
			response:  `{"schema_version":"1","summary":"done","changed_files":[" "]}`,
			sessionID: "ses_1",
		},
		{name: "missing session", response: `{"schema_version":"1","summary":"done","changed_files":[]}`},
		{
			name:      "multiple documents",
			response:  `{"schema_version":"1","summary":"done","changed_files":[]} {}`,
			sessionID: "ses_1",
		},
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

func TestOpenCodeFinalResponseFormattingAcrossRoles(t *testing.T) {
	t.Setenv("OPENCODE_CONFIG_CONTENT", "")
	// Embedded code and fence-like text must survive byte-for-byte in the answer.
	const answer = "<!doctype html>\n<script>const label = `calculator`;</script>\n```json\n{}\n```"
	answerJSON, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	implementation := validImplementationRequest(t)
	repair := validRepairRequest(t)
	review := store.ReviewRequest{Input: repair.Input, Plan: repair.Plan, Implementation: repair.Implementation, Validation: repair.Validation}
	type roleCase struct {
		name, response, want string
		invoke               func(*testing.T, *fakeProcessRunner) (string, error)
	}
	roles := []roleCase{
		{
			name:     "planning",
			want:     answer,
			response: `{"schema_version":"2","action":"answer","answer":` + string(answerJSON) + `,"summary":"done","steps":[],"acceptance_criteria":[]}`,
			invoke: func(t *testing.T, runner *fakeProcessRunner) (string, error) {
				agent, err := NewReadOnlyAgent(runner, DefaultConfig())
				if err != nil {
					return "", err
				}
				result, err := agent.Plan(t.Context(), implementation.Input)
				return result.Answer, err
			},
		},
		{
			name: "review", want: "done",
			response: `{"schema_version":"1","approved":true,"summary":"done","findings":[],"suggestions":[]}`,
			invoke: func(t *testing.T, runner *fakeProcessRunner) (string, error) {
				agent, err := NewReadOnlyAgent(runner, DefaultConfig())
				if err != nil {
					return "", err
				}
				result, err := agent.Review(t.Context(), review)
				return result.Summary, err
			},
		},
	}
	for _, operation := range []string{"implementation", "repair"} {
		roles = append(roles, roleCase{
			name: operation, want: "done",
			response: `{"schema_version":"1","summary":"done","changed_files":[]}`,
			invoke: func(t *testing.T, runner *fakeProcessRunner) (string, error) {
				agent, err := NewImplementer(runner, DefaultConfig())
				if err != nil {
					return "", err
				}
				var result store.ImplementationResult
				if operation == "repair" {
					result, err = agent.ApplyReview(t.Context(), repair)
				} else {
					result, err = agent.Implement(t.Context(), implementation)
				}
				if err == nil && result.AgentSessionID != "ses_original" {
					t.Errorf("session ID = %q; want ses_original", result.AgentSessionID)
				}
				return result.Summary, err
			},
		})
	}
	fence := func(response string) string { return "```json\n" + response + "\n```" }
	for _, role := range roles {
		t.Run(
			role.name,
			func(t *testing.T) {
				response := role.response
				for _, test := range []struct {
					name, text string
					valid      bool
				}{
					{name: "bare JSON", text: response, valid: true},
					{name: "JSON fence", text: fence(response), valid: true},
					{name: "unlabelled fence", text: "```\n" + response + "\n```", valid: true},
					{name: "CRLF and surrounding whitespace", text: " \t\r\n```json\r\n" + response + "\r\n```\r\n\t ", valid: true},
					{name: "prefix prose", text: "Here is the result:\n" + fence(response)},
					{name: "suffix prose", text: fence(response) + "\nDone."},
					{name: "multiple fences", text: fence(response) + "\n" + fence(response)},
					{name: "nested fences", text: fence(fence(response))},
					{name: "missing closing fence", text: "```json\n" + response},
					{name: "missing fence newline", text: "```json" + response + "```"},
					{name: "unsupported language", text: "```javascript\n" + response + "\n```"},
					{name: "empty fenced content", text: fence("")},
					{name: "malformed JSON", text: fence("{")},
					{name: "trailing JSON document", text: fence(response + "\n{}")},
					{name: "duplicate key", text: fence(strings.Replace(response, `"summary":`, `"summary":"ignored","summary":`, 1))},
					{name: "noncanonical key", text: fence(strings.Replace(response, `"summary":`, `"Summary":`, 1))},
					{name: "unknown field", text: fence(strings.TrimSuffix(response, "}") + `,"extra":true}`)},
					{
						name: "unsupported schema",
						text: fence(strings.Replace(response, `"schema_version":"`, `"schema_version":"unsupported`, 1)),
					},
					{name: "null required field", text: fence(strings.Replace(response, `"summary":"done"`, `"summary":null`, 1))},
					{name: "invalid domain value", text: fence(strings.Replace(response, `"summary":"done"`, `"summary":""`, 1))},
				} {
					t.Run(
						test.name,
						func(t *testing.T) {
							runner := &fakeProcessRunner{run: func(_ context.Context, command process.Command) (process.Result, error) {
								line, err := json.Marshal(map[string]any{"type": "text", "sessionID": "ses_original", "part": map[string]string{"type": "text", "text": test.text}})
								if err != nil {
									t.Fatal(err)
								}
								writeOutput(t, command, string(line)+"\n")
								return process.Result{}, nil
							}}
							got, err := role.invoke(t, runner)
							if test.valid {
								if err != nil || got != role.want {
									t.Fatalf("response changed or rejected: got %q, error %v", got, err)
								}
							} else {
								var outputError *OutputError
								if !errors.As(err, &outputError) {
									t.Fatalf("invalid response error = %v; want OutputError", err)
								}
							}
							if runner.calls != 1 {
								t.Fatalf("runner calls = %d; must not retry", runner.calls)
							}
						},
					)
				}
			},
		)
	}
}
