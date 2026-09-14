package main

import (
	"encoding/json"
	"os"
	"strings"
)

// Each protocol emits its native envelope. The application must still execute
// the real validation child and must not reinterpret rejected review decisions.
func teamCompatibility(args []string, prompt string) {
	if len(args) > 0 && args[0] == "verify" {
		team(args, prompt)
		return
	}
	provider := os.Getenv("BDD_PROVIDER")
	behavior := os.Getenv("BDD_REVIEW_OUTPUT")
	response := `{"schema_version":"1","summary":"completed","changed_files":["provider-edit.txt"]}`
	isReview := strings.HasPrefix(prompt, "You are the independent review stage")
	switch {
	case strings.HasPrefix(prompt, "You are the planning stage"):
		response = `{"schema_version":"2","action":"implement","answer":"","summary":"Create the requested file","steps":["Write completed to provider-edit.txt"],"acceptance_criteria":["The requested file contains completed"]}`
	case isReview:
		data, err := os.ReadFile("provider-edit.txt")
		must(err)
		if string(data) != "completed" {
			os.Exit(1)
		}
		response = `{"schema_version":"1","approved":true,"summary":"file independently verified","findings":[],"suggestions":[]}`
	default:
		must(os.WriteFile("provider-edit.txt", []byte("completed"), 0600))
		if behavior == "provider-error" {
			message := "HTTP 400: Unrecognized request argument supplied: prompt_cache_key; private-diagnostic-marker"
			if provider == "claude" {
				must(json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "result", "subtype": "error_during_execution", "is_error": true, "result": message}))
			} else {
				kind := "error"
				if provider == "codex" {
					kind = "turn.failed"
				}
				must(json.NewEncoder(os.Stdout).Encode(map[string]any{"type": kind, "error": map[string]any{"message": message, "statusCode": 400}}))
			}
			return
		}
	}
	if os.Getenv("BDD_VERSION_STYLE") == "integer" {
		response = strings.ReplaceAll(response, `"schema_version":"1"`, `"schema_version":1`)
		response = strings.ReplaceAll(response, `"schema_version":"2"`, `"schema_version":2`)
	}
	if isReview {
		switch behavior {
		case "unknown-version":
			response = strings.Replace(response, `"schema_version":1`, `"schema_version":3`, 1)
		case "string-approval":
			response = strings.Replace(response, `"approved":true`, `"approved":"true"`, 1)
		case "duplicate-approval":
			response = strings.Replace(response, `"approved":true`, `"approved":false,"approved":true`, 1)
		case "blocking-approval":
			response = strings.Replace(response, `"findings":[]`, `"findings":[{"severity":"error","blocking":true,"file":"provider-edit.txt","line":1,"description":"Blocking issue","evidence":"Observed defect","required_action":"Fix it"}]`, 1)
		case "truncated":
			response = strings.TrimSuffix(response, "}")
		}
	}
	switch provider {
	case "codex":
		for i, arg := range args {
			if arg == "--output-last-message" {
				must(os.WriteFile(args[i+1], []byte(response), 0600))
				return
			}
		}
		panic("Codex structured output file was not supplied")
	case "claude":
		// RawMessage deliberately preserves duplicate keys for negative scenarios.
		if isReview && behavior == "truncated" {
			_, err := os.Stdout.Write([]byte(`{"type":"result"`))
			must(err)
			return
		}
		must(json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "result", "subtype": "success", "is_error": false, "structured_output": json.RawMessage(response)}))
	case "opencode":
		for _, event := range []any{
			map[string]any{"type": "step_start", "sessionID": "ses_compat", "part": map[string]string{"type": "step-start"}},
			map[string]any{"type": "text", "sessionID": "ses_compat", "part": map[string]string{"type": "text", "text": response}},
			map[string]any{"type": "step_finish", "sessionID": "ses_compat", "part": map[string]string{"type": "step-finish", "reason": "stop"}},
		} {
			must(json.NewEncoder(os.Stdout).Encode(event))
		}
	default:
		panic("unknown fixture provider")
	}
}
