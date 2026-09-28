package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func Claude(ctx context.Context, runner Runner, cfg Config, request Request) (Response, error) {
	mode := cfg.PermissionMode
	if mode == "" {
		mode = "default"
	}
	args := []string{"--print", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--permission-prompt-tool", "stdio", "--permission-mode", mode}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.Reasoning != "" {
		args = append(args, "--effort", cfg.Reasoning)
	}
	if len(request.Schema) > 0 {
		args = append(args, "--json-schema", string(request.Schema))
	}
	if request.SessionID != "" {
		args = append(args, "--resume="+request.SessionID)
	}
	if !cfg.Direct {
		tools := "Read,Glob,Grep"
		if cfg.CanWrite {
			tools += ",Edit,Write"
		}
		args = append(args, "--no-session-persistence", "--tools", tools, "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--setting-sources", "user,project,local", "--settings", `{"disableAllHooks":true}`, "--disable-slash-commands")
		request.Prompt += "\nShell, MCP, subagents and hooks are unavailable. Use file tools; configured validation runs separately. Do not claim you ran unavailable commands."
	}
	c, err := start(ctx, runner, process.Command{Name: cfg.Executable, Args: args, Dir: request.Directory, Timeout: cfg.Timeout, OutputLimit: 1 << 20})
	if err != nil {
		return Response{}, err
	}
	defer c.close()
	if err = c.send(dict{"type": "control_request", "request_id": "multiharness-init", "request": dict{"subtype": "initialize", "hooks": nil}}); err != nil {
		return Response{}, err
	}
	initialized := false
	userDeclined := false
	response := Response{}
	for {
		m, err := c.next()
		if err != nil {
			return response, err
		}
		switch str(m["type"]) {
		case "control_response":
			r := obj(m["response"])
			if str(r["request_id"]) != "multiharness-init" {
				continue
			}
			if initialized || str(r["subtype"]) != "success" {
				return response, errors.New("Claude control initialization failed")
			}
			initialized = true
			if err = c.send(dict{"type": "user", "session_id": request.SessionID, "message": dict{"role": "user", "content": request.Prompt}, "parent_tool_use_id": nil}); err != nil {
				return response, err
			}
		case "control_request":
			id := str(m["request_id"])
			p := obj(m["request"])
			if id == "" {
				return response, errors.New("Claude approval has no request id")
			}
			if str(p["subtype"]) != "can_use_tool" {
				if err = c.send(dict{"type": "control_response", "response": dict{"subtype": "error", "request_id": id, "error": "Unsupported Multiharness control request"}}); err != nil {
					return response, err
				}
				continue
			}
			approval, values := claudeChoices(p)
			if !cfg.Direct && !cfg.CanWrite {
				choices := approval.Choices[:0]
				for _, ch := range approval.Choices {
					if ch.ID == "once" || ch.ID == "deny" {
						choices = append(choices, ch)
					}
				}
				approval.Choices = choices
			}
			choice := ""
			tool := str(p["tool_name"])
			permitted := cfg.Direct || tool == "Read" || tool == "Glob" || tool == "Grep" || (cfg.CanWrite && (tool == "Write" || tool == "Edit"))
			if initialized && permitted {
				choice, err = c.decide(cfg.Approver, approval, func(n object) bool {
					return (str(n["type"]) == "control_cancel_request" && str(n["request_id"]) == id) || str(n["type"]) == "result"
				})
			}
			if errors.Is(err, errWithdrawn) {
				continue
			}
			if err != nil {
				return response, err
			}
			value := any(dict{"behavior": "deny", "message": "Permission declined in Multiharness"})
			if choice != "" {
				value = values[choice]
			}
			if permitted && (choice == "" || choice == "deny") {
				userDeclined = true
			}
			if err = c.send(dict{"type": "control_response", "response": dict{"subtype": "success", "request_id": id, "response": value}}); err != nil {
				return response, err
			}
		case "result":
			if !initialized {
				return response, errors.New("Claude result arrived before initialization")
			}
			response.SessionID = str(m["session_id"])
			response.Text = str(m["result"])
			response.Data = m["structured_output"]
			if str(m["subtype"]) != "success" || string(m["is_error"]) != "false" {
				return response, provider.Classify(raw(dict{"message": str(m["result"]), "errors": m["errors"]}), time.Now())
			}
			if len(request.Schema) > 0 && len(response.Data) == 0 {
				return response, errors.New("Claude returned no structured result")
			}
			if !cfg.Direct {
				var denials []struct {
					Tool  string `json:"tool_name"`
					Input struct {
						Path string `json:"file_path"`
					} `json:"tool_input"`
				}
				if json.Unmarshal(m["permission_denials"], &denials) == nil && len(denials) > 0 {
					denied := &store.PermissionDenied{Action: store.BlockedAction{Tool: denials[0].Tool, Target: denials[0].Input.Path}, UserDeclined: userDeclined}
					if denied.Validate() == nil {
						return response, denied
					}
					return response, errors.New("Claude reported denied tools")
				}
			}
			return response, nil
		}
	}
}

func claudeChoices(p object) (store.NativeApproval, map[string]any) {
	r := store.NativeApproval{Harness: "Claude", Action: str(p["tool_name"]), Detail: describe(p, "description", "decision_reason", "blocked_path", "input")}
	values := map[string]any{}
	add := func(id, label, scope, rule string, v any) {
		r.Choices = append(r.Choices, store.ApprovalChoice{ID: id, Label: label, Scope: scope, Rule: rule})
		values[id] = v
	}
	add("once", "Allow once", "once", "", dict{"behavior": "allow", "updatedInput": p["input"]})
	if string(p["suppressAlwaysAllowRule"]) != "true" && string(p["suppress_always_allow_rule"]) != "true" {
		var suggestions []json.RawMessage
		_ = json.Unmarshal(p["permission_suggestions"], &suggestions)
		for i, s := range suggestions {
			scope := str(obj(s)["destination"])
			// A suggested native update is relayed unchanged, including its scope.
			if scope == "" {
				continue
			}
			add(fmt.Sprintf("rule-%d", i), "Allow and apply this native permission update", scope, string(s), dict{"behavior": "allow", "updatedInput": p["input"], "updatedPermissions": []json.RawMessage{s}})
		}
	}
	add("deny", "Deny", "once", "", dict{"behavior": "deny", "message": "Permission declined in Multiharness"})
	return r, values
}
