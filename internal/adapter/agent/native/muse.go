package native

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	args := []string{"serve", "--no-session-log"}
	shell := cfg.Shell && cfg.CanWrite && cfg.Approver != nil
	if !shell {
		args = append(args, "--disable-shell")
	}
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
	if shell {
		prompt = request.Prompt + "\nShell commands Muse does not already trust wait for the user's approval; a declined command did not run. Configured validation still runs separately. Return the final response without markdown fences."
	}
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
	// pending maps an open approval to the requirement already decided.
	pending := map[string]string{}
	// declined records the user's refusal; Muse's reject aborts the turn.
	var declined *store.PermissionDenied
	resolve := func(p object) error {
		if p["currentRequirementId"] == nil || str(p["approvalId"]) == "" {
			return errors.New("Muse approval has no requirement identity")
		}
		approval, deny := museChoices(p)
		choice := ""
		var err error
		if cfg.CanWrite {
			choice, err = c.decide(cfg.Approver, approval, func(n object) bool {
				q := obj(n["params"])
				same := str(q["approvalId"]) == str(p["approvalId"])
				moved := same && string(q["currentRequirementId"]) != string(p["currentRequirementId"])
				method := str(n["method"])
				return (method == "approval/resolved" && same) || ((method == "approval/request" || method == "approval/updated") && moved) || (method == "turn/completed" && str(q["sessionId"]) == session && str(q["turnId"]) == turn)
			})
		}
		if errors.Is(err, errWithdrawn) {
			return nil
		}
		if err != nil {
			return err
		}
		if cfg.CanWrite && cfg.Approver != nil && (choice == "" || choice == deny) {
			declined = &store.PermissionDenied{Action: museBlocked(p), UserDeclined: true}
		}
		if choice == "" {
			choice = deny
		}
		if choice == "" {
			return errors.New("Muse offered no denial choice")
		}
		pending[str(p["approvalId"])] = string(p["currentRequirementId"])
		_, err = c.call("approval/decide", dict{"commandId": commandID(), "sessionId": session, "approvalId": p["approvalId"], "requirementId": p["currentRequirementId"], "choiceId": choice})
		return err
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
			pending[str(p["approvalId"])] = ""
			if err = resolve(p); err != nil {
				return response, err
			}
		case "approval/updated":
			// A compound shell command is approved stage by stage: after a
			// non-terminal decision Muse advances currentRequirementId here.
			decided, open := pending[str(p["approvalId"])]
			if open && string(p["currentRequirementId"]) != decided && hasChoices(p) {
				if err = resolve(p); err != nil {
					return response, err
				}
			}
		case "approval/resolved":
			delete(pending, str(p["approvalId"]))
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
				if declined != nil && declined.Validate() == nil {
					return response, declined
				}
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

func hasChoices(p object) bool {
	var choices []json.RawMessage
	return json.Unmarshal(p["availableChoices"], &choices) == nil && len(choices) > 0
}

// museAction names the tool and, for staged shell commands, which stage of
// how many awaits a decision (stage updates omit the tool name).
func museAction(p object) string {
	var subject struct {
		Kind    string            `json:"kind"`
		Command string            `json:"command"`
		Stages  []json.RawMessage `json:"stages"`
	}
	var requirement struct {
		SourceIndex int `json:"sourceIndex"`
	}
	_ = json.Unmarshal(p["subject"], &subject)
	_ = json.Unmarshal(p["currentRequirementId"], &requirement)
	action := str(p["toolName"])
	if action == "" {
		action = subject.Kind
	}
	if len(subject.Stages) > 1 {
		action += fmt.Sprintf(" (stage %d of %d)", requirement.SourceIndex+1, len(subject.Stages))
	}
	return action
}

// museBlocked describes the refused action for the workflow's failure report.
func museBlocked(p object) store.BlockedAction {
	var subject struct {
		Kind, Command, Path, Host string
	}
	_ = json.Unmarshal(p["subject"], &subject)
	tool := str(p["toolName"])
	if tool == "" {
		tool = subject.Kind
	}
	target := subject.Command
	if target == "" {
		target = subject.Path + subject.Host
	}
	if len(target) > 2048 {
		target = target[:2048]
	}
	return store.BlockedAction{Tool: tool, Target: target}
}

func museChoices(p object) (store.NativeApproval, string) {
	r := store.NativeApproval{Harness: "Muse", Action: museAction(p), Detail: describe(p, "subject", "rawArgs")}
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
		// Muse 1.4 labels its reject choice "abort"; it ends the turn.
		if ch.Decision == "denied" || ch.Decision == "abort" {
			deny = ch.ID
		}
	}
	return r, deny
}
