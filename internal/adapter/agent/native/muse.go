package native

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

// Muse command IDs are UUIDv7 idempotency keys, distinct from RPC request IDs.
func commandID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	n := time.Now().UnixMilli()
	for i := 5; i >= 0; i-- {
		b[i] = byte(n)
		n >>= 8
	}
	b[6] = b[6]&15 | 0x70
	b[8] = b[8]&63 | 0x80
	s := hex.EncodeToString(b[:])
	return s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:]
}

func Muse(ctx context.Context, runner Runner, cfg Config, request Request) (Response, error) {
	args := []string{"serve", "--no-session-log", "--disable-shell"}
	if !cfg.CanWrite {
		args = append(args, "--disable-write")
	}
	c, err := start(ctx, runner, process.Command{Name: cfg.Executable, Args: args, Dir: request.Directory, Timeout: cfg.Timeout, OutputLimit: 1 << 20})
	if err != nil {
		return Response{}, err
	}
	defer c.close()
	_, err = c.call("initialize", dict{"clientInfo": dict{"name": "multiharness", "version": "1"}, "capabilities": dict{"userInputDialogs": false}})
	if err != nil {
		return Response{}, err
	}
	if err = c.send(dict{"jsonrpc": "2.0", "method": "initialized", "params": dict{}}); err != nil {
		return Response{}, err
	}
	result, err := c.call("session/start", dict{"commandId": commandID(), "workspaceRoot": request.Directory, "modelId": cfg.Model, "providerId": "meta", "approvalMode": "promptUnmatched"})
	if err != nil {
		return Response{}, err
	}
	session := str(obj(result["session"])["sessionId"])
	if session == "" {
		return Response{}, errors.New("Muse returned no session")
	}
	prompt := request.Prompt + "\nShell execution is unavailable. Use file tools; configured validation runs separately. Return the final response without markdown fences."
	if len(request.Schema) > 0 {
		prompt += "\nYour final response must be a JSON object matching this JSON Schema: " + string(request.Schema)
	}
	result, err = c.call("turn/start", dict{"commandId": commandID(), "sessionId": session, "input": []any{dict{"type": "text", "text": prompt}}, "reasoningEffort": cfg.Reasoning})
	if err != nil {
		return Response{}, err
	}
	turn := str(result["turnId"])
	if turn == "" {
		return Response{}, errors.New("Muse returned no turn")
	}
	response := Response{}
	for {
		m, err := c.next()
		if err != nil {
			return response, err
		}
		p := obj(m["params"])
		method := str(m["method"])
		if str(p["sessionId"]) != session || (p["turnId"] != nil && str(p["turnId"]) != turn) {
			if m["id"] != nil && method != "" {
				return response, errors.New("unexpected Muse request scope")
			}
			continue
		}
		switch method {
		case "approval/request":
			if m["id"] == nil {
				return response, errors.New("Muse approval has no request id")
			}
			if err = c.send(dict{"jsonrpc": "2.0", "id": m["id"], "result": dict{}}); err != nil {
				return response, err
			}
			approval, deny := museChoices(p)
			choice := ""
			if cfg.CanWrite {
				choice, err = c.decide(cfg.Approver, approval, func(n object) bool {
					q := obj(n["params"])
					return (str(n["method"]) == "approval/resolved" && str(q["approvalId"]) == str(p["approvalId"])) || (str(n["method"]) == "approval/request" && str(q["approvalId"]) == str(p["approvalId"]) && string(q["currentRequirementId"]) != string(p["currentRequirementId"])) || (str(n["method"]) == "turn/completed" && str(q["sessionId"]) == session && str(q["turnId"]) == turn)
				})
			}
			if errors.Is(err, errWithdrawn) {
				continue
			}
			if err != nil {
				return response, err
			}
			if choice == "" {
				choice = deny
			}
			if choice == "" {
				return response, errors.New("Muse offered no denial choice")
			}
			if p["currentRequirementId"] == nil || str(p["approvalId"]) == "" {
				return response, errors.New("Muse approval has no requirement identity")
			}
			_, err = c.call("approval/decide", dict{"commandId": commandID(), "sessionId": session, "approvalId": p["approvalId"], "requirementId": p["currentRequirementId"], "choiceId": choice})
			if err != nil {
				return response, err
			}
		case "item/completed":
			item := obj(p["item"])
			if str(item["kind"]) == "agentMessage" && str(item["turnId"]) == turn {
				if string(item["truncated"]) == "true" {
					return response, errors.New("Muse final response was truncated")
				}
				response.Text = str(item["text"])
				response.Data = rawText(response.Text)
			}
		case "turn/completed":
			if str(p["turnId"]) != turn {
				continue
			}
			if str(p["terminal"]) != "completed" {
				if p["error"] != nil {
					return response, provider.Classify(p["error"], time.Now())
				}
				return response, errors.New("Muse turn did not complete")
			}
			if response.Text == "" {
				return response, errors.New("Muse returned no final response")
			}
			return response, nil
		case "command/rejected":
			return response, errors.New("Muse rejected the native permission command")
		default:
			if m["id"] != nil && method != "" {
				return response, errors.New("unsupported Muse native request")
			}
		}
	}
}

func museChoices(p object) (store.NativeApproval, string) {
	r := store.NativeApproval{Harness: "Muse", Action: str(p["toolName"]), Detail: describe(p, "subject", "rawArgs")}
	deny := ""
	var choices []struct {
		ID       string `json:"choiceId"`
		Label    string `json:"label"`
		Scope    string `json:"scope"`
		Rule     string `json:"rulePreview"`
		Decision string `json:"decision"`
	}
	_ = json.Unmarshal(p["availableChoices"], &choices)
	for _, ch := range choices {
		if ch.ID == "" {
			continue
		}
		r.Choices = append(r.Choices, store.ApprovalChoice{ID: ch.ID, Label: ch.Label, Scope: ch.Scope, Rule: ch.Rule})
		if ch.Decision == "denied" {
			deny = ch.ID
		}
	}
	return r, deny
}
