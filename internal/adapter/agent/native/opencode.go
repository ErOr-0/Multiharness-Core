package native

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

// OpenCode uses ACP so permission replies reach the running tool invocation.
// The harness owns files and terminals; no client-side tool execution is exposed.
func OpenCode(ctx context.Context, runner Runner, cfg Config, request Request) (Response, error) {
	args := []string{"acp", "--cwd", request.Directory}
	if !cfg.CanWrite {
		args = append(args, "--pure")
	}
	c, err := start(ctx, runner, process.Command{Name: cfg.Executable, Args: args, Dir: request.Directory, Timeout: cfg.Timeout, EnvOverrides: cfg.Environment, OutputLimit: 1 << 20})
	if err != nil {
		return Response{}, err
	}
	defer c.close()
	_, err = c.call("initialize", dict{"protocolVersion": 1, "clientCapabilities": dict{"fs": dict{"readTextFile": false, "writeTextFile": false}, "terminal": false}, "clientInfo": dict{"name": "multiharness", "version": "1"}})
	if err != nil {
		return Response{}, err
	}
	method := "session/new"
	params := dict{"cwd": request.Directory, "mcpServers": []any{}}
	if request.SessionID != "" {
		method = "session/load"
		params["sessionId"] = request.SessionID
	}
	r, err := c.call(method, params)
	if err != nil {
		return Response{}, err
	}
	session := request.SessionID
	if session == "" {
		session = str(r["sessionId"])
	}
	if session == "" {
		return Response{}, errors.New("OpenCode returned no session")
	}
	for _, setting := range [][2]string{{"model", cfg.Model}, {"effort", cfg.Variant}, {"mode", cfg.Mode}} {
		if setting[1] == "" {
			continue
		}
		_, err = c.call("session/set_config_option", dict{"sessionId": session, "configId": setting[0], "value": setting[1]})
		if err != nil {
			return Response{}, err
		}
	}
	// session/load replays old transcript events before its response. Those are
	// context for the native session, not this invocation's final answer.
	c.queue = nil
	c.queueBytes = 0
	c.seq++
	promptID := "multiharness-prompt"
	if err = c.send(dict{"jsonrpc": "2.0", "id": promptID, "method": "session/prompt", "params": dict{"sessionId": session, "prompt": []any{dict{"type": "text", "text": request.Prompt}}}}); err != nil {
		return Response{}, err
	}
	response := Response{SessionID: session}
	var text strings.Builder
	for {
		m, err := c.next()
		if err != nil {
			return response, err
		}
		method := str(m["method"])
		p := obj(m["params"])
		if method == "" && str(m["id"]) == promptID {
			if m["error"] != nil {
				return response, provider.Classify(m["error"], time.Now())
			}
			if str(obj(m["result"])["stopReason"]) != "end_turn" {
				return response, errors.New("OpenCode turn did not complete")
			}
			response.Text = text.String()
			response.Data = rawText(response.Text)
			if response.Text == "" {
				return response, errors.New("OpenCode returned no final response")
			}
			return response, nil
		}
		if str(p["sessionId"]) != session {
			if m["id"] != nil && method != "" {
				return response, errors.New("unexpected OpenCode request scope")
			}
			continue
		}
		switch method {
		case "session/request_permission":
			if m["id"] == nil {
				return response, errors.New("OpenCode approval has no request id")
			}
			approval := store.NativeApproval{Harness: "OpenCode", Action: str(obj(p["toolCall"])["title"]), Detail: describe(obj(p["toolCall"]), "kind", "rawInput", "locations", "content")}
			var options []struct {
				ID   string `json:"optionId"`
				Name string `json:"name"`
				Kind string `json:"kind"`
			}
			_ = json.Unmarshal(p["options"], &options)
			for _, o := range options {
				if o.ID != "" {
					approval.Choices = append(approval.Choices, store.ApprovalChoice{ID: o.ID, Label: o.Name, Scope: o.Kind})
				}
			}
			choice := ""
			if cfg.CanWrite {
				choice, err = c.decide(cfg.Approver, approval, func(n object) bool { return str(n["method"]) == "" && str(n["id"]) == promptID })
			}
			if errors.Is(err, errWithdrawn) {
				continue
			}
			if err != nil {
				return response, err
			}
			outcome := dict{"outcome": "cancelled"}
			if choice != "" {
				outcome = dict{"outcome": "selected", "optionId": choice}
			}
			if err = c.send(dict{"jsonrpc": "2.0", "id": m["id"], "result": dict{"outcome": outcome}}); err != nil {
				return response, err
			}
		case "session/update":
			u := obj(p["update"])
			kind := str(u["sessionUpdate"])
			if kind == "agent_message_chunk" {
				content := obj(u["content"])
				if str(content["type"]) == "text" {
					s := str(content["text"])
					if text.Len()+len(s) > frameLimit {
						return response, errors.New("OpenCode response exceeds limit")
					}
					text.WriteString(s)
				}
			}
			// Tool use separates intermediate narration from the final answer.
			if kind == "tool_call" {
				text.Reset()
			}
		default:
			if m["id"] != nil && method != "" {
				if err = c.send(dict{"jsonrpc": "2.0", "id": m["id"], "error": dict{"code": -32601, "message": "Unsupported Multiharness client capability"}}); err != nil {
					return response, err
				}
			}
		}
	}
}
