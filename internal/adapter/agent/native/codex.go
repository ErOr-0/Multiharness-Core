package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/contract"
)

type Config struct {
	Executable, Model, Reasoning, Sandbox string
	PermissionMode                        string
	Variant, Mode                         string
	Environment                           map[string]string
	Timeout                               time.Duration
	CanWrite, Direct                      bool
	// Shell lets an approving Muse writer run commands; each command Muse does
	// not already trust becomes a native approval request.
	Shell    bool
	Approver contract.NativeApprover
}

type Request struct {
	Directory, Prompt, SessionID string
	Schema                       json.RawMessage
}
type Response struct {
	Text, SessionID string
	Data            json.RawMessage
}

func Codex(ctx context.Context, runner Runner, cfg Config, request Request) (Response, error) {
	c, err := start(ctx, runner, process.Command{Name: cfg.Executable, Args: []string{"app-server"}, Dir: request.Directory, Timeout: cfg.Timeout, OutputLimit: 1 << 20})
	if err != nil {
		return Response{}, err
	}
	defer c.close()
	_, err = c.call("initialize", dict{"clientInfo": dict{"name": "multiharness", "version": "1"}, "capabilities": dict{"experimentalApi": true}})
	if err != nil {
		return Response{}, err
	}
	if err = c.send(dict{"jsonrpc": "2.0", "method": "initialized", "params": dict{}}); err != nil {
		return Response{}, err
	}
	policy := "on-request"
	if !cfg.CanWrite {
		policy = "never"
	}
	params := dict{"model": cfg.Model, "cwd": request.Directory, "sandbox": cfg.Sandbox, "approvalPolicy": policy, "approvalsReviewer": "user"}
	method := "thread/start"
	if request.SessionID != "" {
		method = "thread/resume"
		params["threadId"] = request.SessionID
	} else {
		params["ephemeral"] = !cfg.Direct
	}
	result, err := c.call(method, params)
	if err != nil {
		return Response{}, err
	}
	thread := str(obj(result["thread"])["id"])
	if thread == "" {
		return Response{}, errors.New("Codex returned no thread")
	}
	turnParams := dict{"threadId": thread, "input": []any{dict{"type": "text", "text": request.Prompt}}, "effort": cfg.Reasoning}
	if len(request.Schema) > 0 {
		turnParams["outputSchema"] = request.Schema
	}
	result, err = c.call("turn/start", turnParams)
	if err != nil {
		return Response{}, err
	}
	turn := str(obj(result["turn"])["id"])
	if turn == "" {
		return Response{}, errors.New("Codex returned no turn")
	}
	response := Response{SessionID: thread}
	items := map[string]json.RawMessage{}
	for {
		m, err := c.next()
		if err != nil {
			return response, err
		}
		method := str(m["method"])
		p := obj(m["params"])
		if str(p["threadId"]) != thread || (p["turnId"] != nil && str(p["turnId"]) != turn) {
			if m["id"] != nil && method != "" {
				return response, errors.New("unexpected Codex request scope")
			}
			continue
		}
		switch method {
		case "item/started", "item/completed":
			item := obj(p["item"])
			id := str(item["id"])
			if method == "item/started" && str(item["type"]) == "fileChange" {
				if len(items) >= 128 {
					return response, errors.New("too many pending file changes")
				}
				items[id] = p["item"]
			}
			if method == "item/completed" {
				delete(items, id)
				if str(item["type"]) == "agentMessage" {
					response.Text = str(item["text"])
					response.Data = rawText(response.Text)
				}
			}
		case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "item/permissions/requestApproval":
			if m["id"] == nil {
				return response, errors.New("Codex approval has no request id")
			}
			approval, values, deny := codexChoices(method, p, items[str(p["itemId"])])
			choice := ""
			if cfg.CanWrite {
				choice, err = c.decide(cfg.Approver, approval, func(n object) bool {
					q := obj(n["params"])
					switch str(n["method"]) {
					case "serverRequest/resolved":
						return string(q["requestId"]) == string(m["id"])
					case "turn/completed":
						return str(q["threadId"]) == thread && str(obj(q["turn"])["id"]) == turn
					}
					return false
				})
			}
			if errors.Is(err, errWithdrawn) {
				continue
			}
			if err != nil {
				return response, err
			}
			value := deny
			if choice != "" {
				value = values[choice]
			}
			if err = c.send(dict{"jsonrpc": "2.0", "id": m["id"], "result": value}); err != nil {
				return response, err
			}
		case "turn/completed":
			t := obj(p["turn"])
			if str(t["id"]) != turn {
				continue
			}
			if str(t["status"]) != "completed" {
				if t["error"] != nil {
					return response, provider.Classify(t["error"], time.Now())
				}
				return response, errors.New("Codex turn did not complete")
			}
			if response.Text == "" {
				return response, errors.New("Codex returned no final response")
			}
			return response, nil
		default:
			if m["id"] != nil && method != "" {
				if err = c.send(dict{"jsonrpc": "2.0", "id": m["id"], "error": dict{"code": -32601, "message": "This request is not supported by Multiharness"}}); err != nil {
					return response, err
				}
			}
		}
	}
}

func rawText(text string) json.RawMessage { return json.RawMessage(text) }

func codexChoices(method string, p object, item json.RawMessage) (contract.NativeApproval, map[string]any, any) {
	action := "Run command"
	if method == "item/fileChange/requestApproval" {
		action = "Change files"
	}
	if method == "item/permissions/requestApproval" {
		action = "Grant additional access"
	}
	request := contract.NativeApproval{Harness: "Codex", Action: action, Detail: describe(p, "reason", "command", "cwd", "grantRoot", "networkApprovalContext", "permissions")}
	if len(item) > 0 {
		request.Detail += "\nChanges: " + string(item)
	}
	values := map[string]any{}
	add := func(id, label, scope, rule string, v any) {
		request.Choices = append(request.Choices, contract.ApprovalChoice{ID: id, Label: label, Scope: scope, Rule: rule})
		values[id] = v
	}
	deny := any(dict{"decision": "decline"})
	if method == "item/permissions/requestApproval" {
		deny = dict{"permissions": dict{}, "scope": "turn"}
		permissions := obj(p["permissions"])
		for k, v := range permissions {
			if string(v) == "null" {
				delete(permissions, k)
			}
		}
		if len(permissions) > 0 {
			add("turn", "Allow requested permissions", "turn", "", dict{"permissions": permissions, "scope": "turn"})
			add("session", "Allow requested permissions", "session", "", dict{"permissions": permissions, "scope": "session"})
		}
	} else {
		add("once", "Allow once", "once", "", dict{"decision": "accept"})
		add("session", "Allow for this native session", "session", "", dict{"decision": "acceptForSession"})
		if method == "item/commandExecution/requestApproval" {
			if v := p["proposedExecpolicyAmendment"]; len(v) > 0 && string(v) != "null" {
				add("rule", "Save the proposed command rule", "persistent", string(v), dict{"decision": dict{"acceptWithExecpolicyAmendment": dict{"execpolicy_amendment": v}}})
			}
			var amendments []json.RawMessage
			_ = json.Unmarshal(p["proposedNetworkPolicyAmendments"], &amendments)
			for i, v := range amendments {
				add(fmt.Sprintf("network-%d", i), "Save the proposed network rule", "persistent", string(v), dict{"decision": dict{"applyNetworkPolicyAmendment": dict{"network_policy_amendment": v}}})
			}
		}
	}
	add("deny", "Deny", "once", "", deny)
	// Newer Codex versions can restrict the offered decisions for a particular
	// command. Do not manufacture choices outside that native list.
	var available []json.RawMessage
	if method == "item/commandExecution/requestApproval" && json.Unmarshal(p["availableDecisions"], &available) == nil && available != nil {
		filtered := request.Choices[:0]
		for _, choice := range request.Choices {
			decision := obj(raw(values[choice.ID]))["decision"]
			for _, offered := range available {
				if string(raw(objOrValue(decision))) == string(raw(objOrValue(offered))) {
					filtered = append(filtered, choice)
					break
				}
			}
		}
		request.Choices = filtered
	}
	return request, values, deny
}

func objOrValue(v json.RawMessage) any { var x any; _ = json.Unmarshal(v, &x); return x }
func describe(p object, keys ...string) string {
	var b strings.Builder
	for _, key := range keys {
		v := p[key]
		if len(v) == 0 || string(v) == "null" {
			continue
		}
		text := str(v)
		if text == "" {
			text = string(v)
		}
		fmt.Fprintf(&b, "%s: %s\n", key, text)
	}
	return b.String()
}

// Model is one selectable catalog entry. Descriptions are provider text and
// must be rendered as content, never as terminal control sequences.
type Model struct {
	ID, Description string
	Default         bool
}

// CodexModels reads the app-server catalog without starting a thread or turn.
// Hidden entries are internal to Codex and are not offered for selection.
func CodexModels(ctx context.Context, runner Runner, executable, dir string) ([]Model, error) {
	c, err := start(ctx, runner, process.Command{Name: executable, Args: []string{"app-server"}, Dir: dir, Timeout: 20 * time.Second, OutputLimit: 256 << 10})
	if err != nil {
		return nil, err
	}
	defer c.close()
	if _, err = c.call("initialize", dict{"clientInfo": dict{"name": "multiharness", "version": "1"}}); err != nil {
		return nil, err
	}
	if err = c.send(dict{"jsonrpc": "2.0", "method": "initialized", "params": dict{}}); err != nil {
		return nil, err
	}
	var models []Model
	cursor := ""
	for range 20 {
		params := dict{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result, err := c.call("model/list", params)
		if err != nil {
			return nil, err
		}
		var page []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Hidden      bool   `json:"hidden"`
			Default     bool   `json:"isDefault"`
		}
		if err := json.Unmarshal(result["data"], &page); err != nil {
			return nil, errors.New("Codex returned an unreadable model list")
		}
		for _, m := range page {
			if m.ID != "" && !m.Hidden {
				models = append(models, Model{m.ID, m.Description, m.Default})
			}
		}
		if cursor = str(result["nextCursor"]); cursor == "" {
			return models, nil
		}
	}
	return nil, errors.New("Codex model list has too many pages")
}
