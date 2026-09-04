package opencode

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

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
			name: "planning", want: answer,
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
				agent, err := NewImplementer(runner, DefaultConfig(), nil)
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
		t.Run(role.name, func(t *testing.T) {
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
				{name: "unsupported schema", text: fence(strings.Replace(response, `"schema_version":"`, `"schema_version":"unsupported`, 1))},
				{name: "null required field", text: fence(strings.Replace(response, `"summary":"done"`, `"summary":null`, 1))},
				{name: "invalid domain value", text: fence(strings.Replace(response, `"summary":"done"`, `"summary":""`, 1))},
			} {
				t.Run(test.name, func(t *testing.T) {
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
				})
			}
		})
	}
}
