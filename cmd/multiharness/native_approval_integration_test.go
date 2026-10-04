package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

type nativeFixtureMap = map[string]any

func fixtureNativeProtocol(operation string) error {
	dec, enc := json.NewDecoder(os.Stdin), json.NewEncoder(os.Stdout)
	read := func() (nativeFixtureMap, error) { var m nativeFixtureMap; err := dec.Decode(&m); return m, err }
	write := func(v any) error { return enc.Encode(v) }
	get := func(m nativeFixtureMap, key string) nativeFixtureMap { v, _ := m[key].(map[string]any); return v }
	var prompt string
	for {
		m, err := read()
		if err != nil {
			return err
		}
		p := get(m, "params")
		if operation == "--print" {
			if m["type"] == "control_request" {
				if err = write(nativeFixtureMap{"type": "control_response", "response": nativeFixtureMap{"subtype": "success", "request_id": m["request_id"], "response": nativeFixtureMap{}}}); err != nil {
					return err
				}
				continue
			}
			prompt, _ = get(m, "message")["content"].(string)
			break
		}
		method, _ := m["method"].(string)
		result := nativeFixtureMap{}
		switch method {
		case "initialized":
			continue
		case "thread/start":
			result = nativeFixtureMap{"thread": nativeFixtureMap{"id": "thread"}}
		case "session/start":
			result = nativeFixtureMap{"session": nativeFixtureMap{"sessionId": "session"}}
		case "turn/start":
			parts, _ := p["input"].([]any)
			if len(parts) != 1 {
				return errors.New("fixture prompt missing")
			}
			part, _ := parts[0].(map[string]any)
			prompt, _ = part["text"].(string)
			if operation == "app-server" {
				result = nativeFixtureMap{"turn": nativeFixtureMap{"id": "turn"}}
			} else {
				result = nativeFixtureMap{"turnId": "turn"}
			}
		}
		if method != "session/prompt" {
			if err = write(nativeFixtureMap{"jsonrpc": "2.0", "id": m["id"], "result": result}); err != nil {
				return err
			}
		}
		if prompt != "" {
			break
		}
	}
	if err := fixtureHandoff([]byte(prompt)); err != nil {
		return err
	}
	role := "plan"
	if strings.Contains(prompt, "Implementation request:") {
		role = "implement"
	}
	if strings.Contains(prompt, "Repair request:") {
		role = "repair"
	}
	if strings.Contains(prompt, "Review request:") {
		role = "review"
	}
	if err := fixtureLog("native-" + role); err != nil {
		return err
	}
	var response any
	if role == "implement" || role == "repair" {
		p := nativeFixtureMap{"threadId": "thread", "sessionId": "session", "turnId": "turn", "itemId": "write", "reason": "Write result.txt"}
		request := nativeFixtureMap{"jsonrpc": "2.0", "id": "approval", "method": "item/fileChange/requestApproval", "params": p}
		switch operation {
		case "--print":
			request = nativeFixtureMap{"type": "control_request", "request_id": "approval", "request": nativeFixtureMap{"subtype": "can_use_tool", "tool_name": "Write", "input": nativeFixtureMap{"file_path": "result.txt"}}}
		case "serve":
			request["method"] = "approval/request"
			p["approvalId"] = "approval"
			p["currentRequirementId"] = nativeFixtureMap{"approvalId": "approval", "sourceIndex": 1}
			p["toolName"] = "write_file"
			p["availableChoices"] = []any{nativeFixtureMap{"choiceId": "once", "label": "Allow once", "scope": "once", "decision": "approved"}, nativeFixtureMap{"choiceId": "deny", "label": "Deny", "scope": "once", "decision": "denied"}}
		}
		if err := write(request); err != nil {
			return err
		}
		m, err := read()
		if err != nil {
			return err
		}
		allowed := false
		switch operation {
		case "app-server":
			allowed = m["id"] == "approval" && get(m, "result")["decision"] == "accept"
		case "--print":
			r := get(m, "response")
			allowed = r["request_id"] == "approval" && get(r, "response")["behavior"] == "allow"
		case "serve":
			if m["id"] != "approval" {
				return errors.New("Muse receipt missing")
			}
			m, err = read()
			if err != nil {
				return err
			}
			allowed = get(m, "params")["choiceId"] == "once"
			if err = write(nativeFixtureMap{"id": m["id"], "result": nativeFixtureMap{"status": "accepted", "terminal": true}}); err != nil {
				return err
			}
		}
		if !allowed {
			return errors.New("fixture did not receive native permission")
		}
		content := "broken\n"
		if role == "repair" {
			content = "fixed\n"
		}
		if err = os.WriteFile("result.txt", []byte(content), 0600); err != nil {
			return err
		}
		response = nativeFixtureMap{"schema_version": "1", "summary": "native fixture applied", "changed_files": []string{"result.txt"}}
	} else if role == "plan" {
		response = fixturePlan([]byte(prompt))
	} else {
		var err error
		response, err = fixtureReview()
		if err != nil {
			return err
		}
	}
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	switch operation {
	case "--print":
		return write(nativeFixtureMap{"type": "result", "subtype": "success", "is_error": false, "structured_output": response})
	default:
		item := nativeFixtureMap{"type": "agentMessage", "kind": "agentMessage", "turnId": "turn", "text": string(data)}
		if err = write(nativeFixtureMap{"method": "item/completed", "params": nativeFixtureMap{"threadId": "thread", "sessionId": "session", "turnId": "turn", "item": item}}); err != nil {
			return err
		}
		return write(nativeFixtureMap{"method": "turn/completed", "params": nativeFixtureMap{"threadId": "thread", "sessionId": "session", "turnId": "turn", "terminal": "completed", "turn": nativeFixtureMap{"id": "turn", "status": "completed"}}})
	}
}

type fixtureNativeApprover func(context.Context, contract.NativeApproval) (string, error)

func (f fixtureNativeApprover) ApproveNative(ctx context.Context, r contract.NativeApproval) (string, error) {
	return f(ctx, r)
}

func TestWorkflowNativePermissionIntegration(t *testing.T) {
	for _, harness := range []string{"codex", "claude", "muse"} {
		t.Run(harness, func(t *testing.T) {
			cfg, log := fixtureConfiguration(t)
			helper := cfg.Planner.Executable
			cfg.Planner = config.DefaultPlanner(harness)
			cfg.Planner.Executable = helper
			cfg.Implementer = config.DefaultImplementer(harness)
			cfg.Implementer.Executable = helper
			cfg.Reviewer = config.DefaultPlanner(harness)
			cfg.Reviewer.Executable = helper
			t.Setenv("MULTIHARNESS_FIXTURE_HANDOFF", "1")
			prompts := 0
			deps, err := composeDependencies(cfg, nil, nil, nil, "", fixtureNativeApprover(func(_ context.Context, r contract.NativeApproval) (string, error) { prompts++; return "once", nil }))
			if err != nil {
				t.Fatal(err)
			}
			svc, err := workflow.NewService(deps)
			if err != nil {
				t.Fatal(err)
			}
			r := svc.Run(t.Context(), contract.TaskInput{Task: "fixture change with prior constraints", WorkingDir: cfg.WorkingDir, MaxRepairAttempts: 1, RecentTurns: []contract.ConversationTurn{{User: "Keep the public API unchanged", Assistant: "Preserve the original result format"}}})
			if r.Status != contract.TaskStatusApproved || prompts != 2 || r.AgentInvocations != 5 {
				t.Fatalf("status %s prompts %d invocations %d failure %+v", r.Status, prompts, r.AgentInvocations, r.Failure)
			}
			calls, err := os.ReadFile(log)
			if err != nil || string(calls) != "native-plan\nnative-implement\ncheck\nnative-review\nnative-repair\ncheck\nnative-review\n" {
				t.Fatal(string(calls), err)
			}
		})
	}
}
