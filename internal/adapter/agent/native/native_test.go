package native

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

type approveFunc func(context.Context, store.NativeApproval) (string, error)

func (f approveFunc) ApproveNative(ctx context.Context, r store.NativeApproval) (string, error) {
	return f(ctx, r)
}

type runnerFunc func(context.Context, process.Command) (process.Result, error)

func (f runnerFunc) Run(ctx context.Context, c process.Command) (process.Result, error) {
	return f(ctx, c)
}

type peer struct {
	t   *testing.T
	dec *json.Decoder
	out io.Writer
}

func (p peer) read() object {
	p.t.Helper()
	var m object
	if err := p.dec.Decode(&m); err != nil {
		p.t.Error(err)
		return nil
	}
	return m
}
func (p peer) write(v any) {
	p.t.Helper()
	if _, err := p.out.Write(append(raw(v), '\n')); err != nil {
		p.t.Error(err)
	}
}
func (p peer) rpc(method string, result any) object {
	p.t.Helper()
	m := p.read()
	if str(m["method"]) != method {
		p.t.Errorf("want %s got %s", method, m["method"])
	}
	p.write(dict{"jsonrpc": "2.0", "id": m["id"], "result": result})
	return obj(m["params"])
}
func (p peer) notification(method string, params any) {
	p.write(dict{"jsonrpc": "2.0", "method": method, "params": params})
}
func fixture(t *testing.T, f func(peer, process.Command)) Runner {
	return runnerFunc(func(ctx context.Context, c process.Command) (process.Result, error) {
		f(peer{t, json.NewDecoder(c.Stdin), c.Stdout}, c)
		if len(c.Args) > 0 && c.Args[0] == "--print" {
			_, err := io.Copy(io.Discard, c.Stdin)
			return process.Result{}, err
		}
		// Server processes remain alive after a turn; cancellation must reap them.
		<-ctx.Done()
		return process.Result{ExitCode: 0}, ctx.Err()
	})
}

func codexStartup(p peer) {
	p.rpc("initialize", dict{})
	if str(p.read()["method"]) != "initialized" {
		p.t.Error("no initialized")
	}
	p.rpc("thread/start", dict{"thread": dict{"id": "thread-1"}})
	p.rpc("turn/start", dict{"turn": dict{"id": "turn-1"}})
}
func codexFinish(p peer) {
	p.notification("item/completed", dict{"threadId": "thread-1", "turnId": "turn-1", "item": dict{"type": "agentMessage", "text": `{"done":true}`}})
	p.notification("turn/completed", dict{"threadId": "thread-1", "turn": dict{"id": "turn-1", "status": "completed"}})
}

func TestCodexLiveApprovals(t *testing.T) {
	for _, choice := range []string{"once", "session", "rule", "deny", ""} {
		t.Run(choice, func(t *testing.T) {
			calls := 0
			approvals := 0
			runner := fixture(t, func(p peer, c process.Command) {
				calls++
				if strings.Join(c.Args, " ") != "app-server" {
					t.Error(c.Args)
				}
				codexStartup(p)
				p.write(dict{"id": 19, "method": "item/commandExecution/requestApproval", "params": dict{"threadId": "thread-1", "turnId": "turn-1", "itemId": "item-1", "command": "write target.txt", "proposedExecpolicyAmendment": []string{"write", "target.txt"}}})
				m := p.read()
				if string(m["id"]) != "19" {
					t.Error("wrong request ID")
				}
				decision := obj(m["result"])["decision"]
				want := map[string]string{"once": `"accept"`, "session": `"acceptForSession"`, "deny": `"decline"`, "": `"decline"`, "rule": `{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":["write","target.txt"]}}`}[choice]
				if string(decision) != want {
					t.Errorf("decision %s want %s", decision, want)
				}
				codexFinish(p)
			})
			cfg := Config{Executable: "fixture", Sandbox: "workspace-write", CanWrite: true, Timeout: time.Second * 3, Approver: approveFunc(func(_ context.Context, r store.NativeApproval) (string, error) {
				approvals++
				if !strings.Contains(r.Detail, "target.txt") {
					t.Error(r)
				}
				return choice, nil
			})}
			response, err := Codex(t.Context(), runner, cfg, Request{Prompt: "implement"})
			if err != nil || response.Text != `{"done":true}` || calls != 1 || approvals != 1 {
				t.Fatal(response, err, calls, approvals)
			}
		})
	}
}

func TestCodexReadOnlyCannotEscalate(t *testing.T) {
	runner := fixture(t, func(p peer, c process.Command) {
		codexStartup(p)
		p.write(dict{"id": "readonly", "method": "item/fileChange/requestApproval", "params": dict{"threadId": "thread-1", "turnId": "turn-1", "itemId": "file"}})
		if str(obj(p.read()["result"])["decision"]) != "decline" {
			t.Error("read-only approved")
		}
		codexFinish(p)
	})
	_, err := Codex(t.Context(), runner, Config{Executable: "fixture", Timeout: 3 * time.Second, Approver: approveFunc(func(context.Context, store.NativeApproval) (string, error) {
		t.Error("read-only prompted for write")
		return "once", nil
	})}, Request{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWithdrawnApprovalCancelsPromptAndNeverReplies(t *testing.T) {
	shown := make(chan struct{})
	cancelled := make(chan struct{})
	runner := fixture(t, func(p peer, c process.Command) {
		codexStartup(p)
		p.write(dict{"id": 9, "method": "item/fileChange/requestApproval", "params": dict{"threadId": "thread-1", "turnId": "turn-1", "itemId": "file"}})
		<-shown
		p.notification("serverRequest/resolved", dict{"threadId": "thread-1", "requestId": 9})
		<-cancelled
		codexFinish(p)
		var extra object
		err := p.dec.Decode(&extra)
		if err == nil {
			t.Errorf("stale approval sent: %s", raw(extra))
		}
	})
	_, err := Codex(t.Context(), runner, Config{Executable: "fixture", CanWrite: true, Timeout: 3 * time.Second, Approver: approveFunc(func(ctx context.Context, _ store.NativeApproval) (string, error) {
		close(shown)
		<-ctx.Done()
		close(cancelled)
		return "once", nil
	})}, Request{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestClaudeRuleUpdateAndContinuation(t *testing.T) {
	suggestion := dict{"type": "addRules", "rules": []any{dict{"toolName": "Write", "ruleContent": "/tmp/project/**"}}, "behavior": "allow", "destination": "localSettings"}
	runner := fixture(t, func(p peer, c process.Command) {
		args := strings.Join(c.Args, " ")
		if !strings.Contains(args, "--permission-prompt-tool stdio") || strings.Contains(args, "dontAsk") || strings.Contains(args, "--allowedTools") {
			t.Error(args)
		}
		init := p.read()
		p.write(dict{"type": "control_response", "response": dict{"subtype": "success", "request_id": init["request_id"], "response": dict{}}})
		if str(p.read()["type"]) != "user" {
			t.Error("missing user prompt")
		}
		p.write(dict{"type": "control_request", "request_id": "permission-1", "request": dict{"subtype": "can_use_tool", "tool_name": "Write", "input": dict{"file_path": "/tmp/project/a.go", "content": "package a"}, "permission_suggestions": []any{suggestion}}})
		m := obj(p.read()["response"])
		if str(m["request_id"]) != "permission-1" {
			t.Error("wrong request")
		}
		r := obj(m["response"])
		if str(r["behavior"]) != "allow" || string(r["updatedPermissions"]) != string(raw([]any{suggestion})) {
			t.Error(string(raw(r)))
		}
		if str(obj(r["updatedInput"])["content"]) != "package a" {
			t.Error("input changed")
		}
		p.write(dict{"type": "result", "subtype": "success", "is_error": false, "structured_output": dict{"done": true}, "session_id": "claude-1"})
	})
	r, err := Claude(t.Context(), runner, Config{Executable: "fixture", CanWrite: true, Timeout: 3 * time.Second, Approver: approveFunc(func(_ context.Context, r store.NativeApproval) (string, error) {
		if r.Choices[1].Scope != "localSettings" {
			t.Error(r)
		}
		return "rule-0", nil
	})}, Request{Schema: raw(dict{"type": "object"})})
	if err != nil || string(r.Data) != `{"done":true}` {
		t.Fatal(r, err)
	}
}

func TestMuseNativeChoiceAndRequirementGuard(t *testing.T) {
	runner := fixture(t, func(p peer, c process.Command) {
		p.rpc("initialize", dict{})
		p.read()
		p.rpc("session/start", dict{"session": dict{"sessionId": "muse-1"}})
		p.rpc("turn/start", dict{"turnId": "turn-1"})
		requirement := dict{"approvalId": "approval-1", "sourceIndex": 17}
		p.write(dict{"jsonrpc": "2.0", "id": "server-1", "method": "approval/request", "params": dict{"sessionId": "muse-1", "turnId": "turn-1", "approvalId": "approval-1", "currentRequirementId": requirement, "toolName": "write_file", "availableChoices": []any{dict{"choiceId": "persist-specific", "label": "Save this rule", "scope": "localPersistent", "decision": "approvedPolicyAmendment", "rulePreview": "write /project/a.go"}, dict{"choiceId": "reject", "label": "Deny", "scope": "once", "decision": "denied"}}}})
		receipt := p.read()
		if str(receipt["id"]) != "server-1" || string(receipt["result"]) != "{}" {
			t.Error("missing presentation receipt")
		}
		m := p.rpc("approval/decide", dict{"status": "accepted", "terminal": true})
		if str(m["choiceId"]) != "persist-specific" || string(m["requirementId"]) != string(raw(requirement)) {
			t.Error(string(raw(m)))
		}
		id := str(m["commandId"])
		if len(id) != 36 || id[14] != '7' {
			t.Error("invalid command id", id)
		}
		p.notification("approval/resolved", dict{"sessionId": "muse-1", "approvalId": "approval-1"})
		p.notification("item/completed", dict{"sessionId": "muse-1", "item": dict{"kind": "agentMessage", "turnId": "turn-1", "text": `{"done":true}`}})
		p.notification("turn/completed", dict{"sessionId": "muse-1", "turnId": "turn-1", "terminal": "completed"})
	})
	r, err := Muse(t.Context(), runner, Config{Executable: "fixture", CanWrite: true, Timeout: 3 * time.Second, Approver: approveFunc(func(_ context.Context, r store.NativeApproval) (string, error) {
		if r.Choices[0].Rule != "write /project/a.go" {
			t.Error(r)
		}
		return "persist-specific", nil
	})}, Request{})
	if err != nil || r.Text != `{"done":true}` {
		t.Fatal(r, err)
	}
}

func TestApprovalCancellationStopsNativeProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runner := fixture(t, func(p peer, c process.Command) {
		codexStartup(p)
		p.write(dict{"id": 5, "method": "item/fileChange/requestApproval", "params": dict{"threadId": "thread-1", "turnId": "turn-1", "itemId": "file"}})
	})
	_, err := Codex(ctx, runner, Config{Executable: "fixture", CanWrite: true, Timeout: 3 * time.Second, Approver: approveFunc(func(ctx context.Context, _ store.NativeApproval) (string, error) {
		cancel()
		<-ctx.Done()
		return "once", nil
	})}, Request{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestFramesFailClosed(t *testing.T) {
	for _, data := range []string{"not json\n", strings.Repeat("x", frameLimit+1)} {
		f := frames{ch: make(chan object, 1)}
		if _, err := f.Write([]byte(data)); err == nil {
			t.Fatal("accepted malformed/oversized frame")
		}
	}
	f := frames{ch: make(chan object, 1)}
	if _, err := f.Write([]byte("{}\n{}\n")); err == nil {
		t.Fatal("accepted queue overflow")
	}
}

func TestOpenCodeApprovalResumesPendingPrompt(t *testing.T) {
	runner := fixture(t, func(p peer, c process.Command) {
		p.rpc("initialize", dict{"protocolVersion": 1})
		p.rpc("session/new", dict{"sessionId": "ses_native"})
		model := p.rpc("session/set_config_option", dict{})
		if str(model["configId"]) != "model" || str(model["value"]) != "provider/model" {
			t.Error(model)
		}
		prompt := p.read()
		if str(prompt["method"]) != "session/prompt" {
			t.Error(prompt)
		}
		p.write(dict{"id": "perm-1", "method": "session/request_permission", "params": dict{"sessionId": "ses_native", "toolCall": dict{"title": "Write a.go"}, "options": []any{dict{"optionId": "native-once", "name": "Allow once", "kind": "allow_once"}, dict{"optionId": "native-deny", "name": "Reject", "kind": "reject_once"}}}})
		m := p.read()
		outcome := obj(obj(m["result"])["outcome"])
		if str(m["id"]) != "perm-1" || str(outcome["optionId"]) != "native-once" {
			t.Error(m)
		}
		p.notification("session/update", dict{"sessionId": "ses_native", "update": dict{"sessionUpdate": "agent_message_chunk", "content": dict{"type": "text", "text": `{"done":true}`}}})
		p.write(dict{"id": prompt["id"], "result": dict{"stopReason": "end_turn"}})
	})
	r, err := OpenCode(t.Context(), runner, Config{Executable: "fixture", Model: "provider/model", CanWrite: true, Timeout: 3 * time.Second, Approver: approveFunc(func(context.Context, store.NativeApproval) (string, error) { return "native-once", nil })}, Request{Prompt: "implement"})
	if err != nil || r.Text != `{"done":true}` || r.SessionID != "ses_native" {
		t.Fatal(r, err)
	}
}

func TestCodexRequestedPermissionSubsetAndNativeChoices(t *testing.T) {
	r, values, _ := codexChoices("item/permissions/requestApproval", object{"permissions": raw(dict{"network": nil, "fileSystem": dict{"write": []string{"/project/a"}}})}, nil)
	if len(r.Choices) != 3 {
		t.Fatal(r)
	}
	grant := obj(obj(raw(values["turn"]))["permissions"])
	if grant["network"] != nil || string(grant["fileSystem"]) != `{"write":["/project/a"]}` {
		t.Fatal(string(raw(grant)))
	}
	r, _, _ = codexChoices("item/commandExecution/requestApproval", object{"availableDecisions": raw([]string{"accept", "decline"})}, nil)
	for _, c := range r.Choices {
		if c.ID == "session" {
			t.Fatal("invented unavailable session grant")
		}
	}
}
