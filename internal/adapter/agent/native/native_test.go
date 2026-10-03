package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
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
	big := strings.Repeat("x", frameLimit)
	for _, data := range []string{
		"not json\n",
		strings.Repeat("x", frameLimit+1),
		`{"id":7,"method":"item/fileChange/requestApproval","params":{"diff":"` + big + "\"}}\n",
		`{"type":"result","result":"` + big + "\"}\n",
	} {
		f := frames{ch: make(chan object, 1)}
		if _, err := f.Write([]byte(data)); err == nil {
			t.Fatal("accepted malformed or essential oversized frame")
		}
	}
	// A full queue waits for the reader and fails only once the connection closes.
	done := make(chan struct{})
	f := frames{ch: make(chan object, 1), done: done}
	written := make(chan error, 1)
	go func() { _, err := f.Write([]byte("{}\n{}\n")); written <- err }()
	select {
	case err := <-written:
		t.Fatalf("queue overflow returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(done)
	if err := <-written; err == nil {
		t.Fatal("blocked write survived connection close")
	}
}

// A tool echoing a large file must not abort the run; only the notification is lost.
func TestFramesSkipOversizedNotifications(t *testing.T) {
	big := strings.Repeat("y", frameLimit)
	f := frames{ch: make(chan object, 4)}
	stream := `{"jsonrpc":"2.0","method":"session/update","params":{"text":"` + big + "\"}}\n" +
		`{"type":"user","message":{"content":"` + big + "\"}}\n" +
		`{"id":1,"result":{}}` + "\n"
	// Deliver in pipe-sized pieces so skipping spans multiple writes.
	for data := []byte(stream); len(data) > 0; {
		n := min(len(data), 64<<10)
		if _, err := f.Write(data[:n]); err != nil {
			t.Fatal(err)
		}
		data = data[n:]
	}
	if f.skipped != 2 || len(f.ch) != 1 || string((<-f.ch)["id"]) != "1" {
		t.Fatalf("skipped=%d queued=%d", f.skipped, len(f.ch))
	}
}

// Resumed repair sessions replay their whole transcript before session/load
// responds; the replay must not overflow the pending-event queue.
func TestOpenCodeSessionLoadDiscardsReplay(t *testing.T) {
	runner := fixture(t, func(p peer, c process.Command) {
		p.rpc("initialize", dict{"protocolVersion": 1})
		load := p.read()
		if str(load["method"]) != "session/load" {
			t.Error(load)
		}
		for i := range 1000 {
			p.notification("session/update", dict{"sessionId": "ses_old", "update": dict{"sessionUpdate": "agent_message_chunk", "content": dict{"type": "text", "text": fmt.Sprintf("old %d", i)}}})
		}
		p.write(dict{"jsonrpc": "2.0", "id": load["id"], "result": dict{}})
		prompt := p.read()
		p.notification("session/update", dict{"sessionId": "ses_old", "update": dict{"sessionUpdate": "agent_message_chunk", "content": dict{"type": "text", "text": "fixed"}}})
		p.write(dict{"id": prompt["id"], "result": dict{"stopReason": "end_turn"}})
	})
	r, err := OpenCode(t.Context(), runner, Config{Executable: "fixture", CanWrite: true, Timeout: 3 * time.Second}, Request{Prompt: "repair", SessionID: "ses_old"})
	if err != nil || r.Text != "fixed" {
		t.Fatal(r, err)
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

// Muse keeps its shell disabled unless an interactive writer opted in; with
// the shell on, a command approval travels through the same approver.
func TestMuseShellOnlyForApprovingWriter(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cfg      Config
		disabled bool
	}{
		{"default writer", Config{CanWrite: true}, true},
		{"reader cannot opt in", Config{Shell: true}, true},
		{"unattended cannot opt in", Config{CanWrite: true, Shell: true}, true},
		{"approving writer", Config{CanWrite: true, Shell: true, Approver: approveFunc(func(context.Context, store.NativeApproval) (string, error) { return "allow_once", nil })}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			var prompt string
			runner := fixture(t, func(p peer, c process.Command) {
				args = c.Args
				p.rpc("initialize", dict{})
				p.read()
				p.rpc("session/start", dict{"session": dict{"sessionId": "muse-1"}})
				turn := p.read()
				var params struct {
					Input []struct{ Text string } `json:"input"`
				}
				if json.Unmarshal(turn["params"], &params) == nil && len(params.Input) == 1 {
					prompt = params.Input[0].Text
				}
				p.write(dict{"jsonrpc": "2.0", "id": turn["id"], "result": dict{"turnId": "turn-1"}})
				if !tc.disabled {
					requirement := dict{"approvalId": "a-1", "sourceIndex": 0}
					p.write(dict{"jsonrpc": "2.0", "id": "server-1", "method": "approval/request", "params": dict{"sessionId": "muse-1", "turnId": "turn-1", "approvalId": "a-1", "currentRequirementId": requirement, "toolName": "bash", "subject": dict{"kind": "shell", "command": "go test ./..."}, "availableChoices": []any{dict{"choiceId": "allow_once", "label": "Allow once", "decision": "approved"}, dict{"choiceId": "abort", "label": "Reject", "decision": "abort"}}}})
					p.read()
					if m := p.rpc("approval/decide", dict{}); str(m["choiceId"]) != "allow_once" {
						t.Error(string(raw(m)))
					}
				}
				p.notification("item/completed", dict{"sessionId": "muse-1", "item": dict{"kind": "agentMessage", "turnId": "turn-1", "text": "done"}})
				p.notification("turn/completed", dict{"sessionId": "muse-1", "turnId": "turn-1", "terminal": "completed"})
			})
			cfg := tc.cfg
			cfg.Executable, cfg.Timeout = "fixture", 3*time.Second
			if _, err := Muse(t.Context(), runner, cfg, Request{Prompt: "task"}); err != nil {
				t.Fatal(err)
			}
			if slices.Contains(args, "--disable-shell") != tc.disabled {
				t.Fatalf("args %v", args)
			}
			if strings.Contains(prompt, "Shell execution is unavailable") != tc.disabled {
				t.Fatalf("prompt does not match shell availability: %q", prompt)
			}
		})
	}
}

func TestOpenCodeConfirmAgentAsksForEverythingButReads(t *testing.T) {
	if _, err := WithConfirmAgent(Config{}); !errors.Is(err, ErrConfirmNeedsTerminal) {
		t.Fatalf("unattended confirm = %v", err)
	}
	t.Setenv("OPENCODE_CONFIG_CONTENT", `{"model":"provider/model","agent":{"build":{"permission":{"edit":"allow"}}}}`)
	approver := approveFunc(func(context.Context, store.NativeApproval) (string, error) { return "", nil })
	cfg, err := WithConfirmAgent(Config{Approver: approver, Environment: map[string]string{"KEEP": "1"}})
	if err != nil || !strings.HasPrefix(cfg.Mode, "multiharness-confirm-") || cfg.Environment["KEEP"] != "1" {
		t.Fatal(cfg, err)
	}
	var content struct {
		Model string `json:"model"`
		Agent map[string]struct {
			Mode       string            `json:"mode"`
			Permission map[string]string `json:"permission"`
		} `json:"agent"`
	}
	if err := json.Unmarshal([]byte(cfg.Environment["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
		t.Fatal(err)
	}
	agent := content.Agent[cfg.Mode]
	if content.Model != "provider/model" || content.Agent["build"].Permission["edit"] != "allow" || agent.Mode != "primary" ||
		agent.Permission["*"] != "ask" || agent.Permission["read"] != "allow" || agent.Permission["edit"] != "" {
		t.Fatalf("confirm agent config = %+v", content)
	}
}

// Recorded Muse 1.4.2 traffic: a compound shell command is approved stage by
// stage; after each non-terminal decide, approval/updated names the next stage.
func TestMuseCompoundCommandAsksForEveryStage(t *testing.T) {
	stages := []any{dict{"argv": []string{"mkdir", "-p", "a"}}, dict{"argv": []string{"rm", "-rf", "a"}}, dict{"argv": []string{"touch", "done.txt"}}}
	subject := dict{"kind": "shell", "command": "mkdir -p a && rm -rf a && touch done.txt", "stages": stages}
	choices := []any{dict{"choiceId": "allow_once", "label": "Allow once", "decision": "approved"}, dict{"choiceId": "abort", "label": "Reject", "decision": "abort"}}
	req := func(i int) dict { return dict{"approvalId": "a-1", "sourceIndex": i} }
	runner := fixture(t, func(p peer, c process.Command) {
		p.rpc("initialize", dict{})
		p.read()
		p.rpc("session/start", dict{"session": dict{"sessionId": "muse-1"}})
		p.rpc("turn/start", dict{"turnId": "turn-1"})
		p.write(dict{"jsonrpc": "2.0", "id": "server-1", "method": "approval/request", "params": dict{"sessionId": "muse-1", "turnId": "turn-1", "approvalId": "a-1", "currentRequirementId": req(0), "toolName": "bash", "subject": subject, "availableChoices": choices}})
		p.read()
		for i := range 3 {
			m := p.rpc("approval/decide", dict{"status": "accepted", "approvalId": "a-1", "terminal": i == 2})
			if string(m["requirementId"]) != string(raw(req(i))) {
				t.Errorf("stage %d decided %s", i, m["requirementId"])
			}
			next := min(i+1, 2)
			p.notification("approval/updated", dict{"sessionId": "muse-1", "approvalId": "a-1", "change": dict{"kind": "stageResolved", "requirementId": req(i)}, "currentRequirementId": req(next), "subject": subject, "availableChoices": choices})
		}
		p.notification("approval/resolved", dict{"sessionId": "muse-1", "approvalId": "a-1", "resolvedBy": "user"})
		p.notification("item/completed", dict{"sessionId": "muse-1", "item": dict{"kind": "agentMessage", "turnId": "turn-1", "text": "done"}})
		p.notification("turn/completed", dict{"sessionId": "muse-1", "turnId": "turn-1", "terminal": "completed"})
	})
	var asked []string
	_, err := Muse(t.Context(), runner, Config{Executable: "fixture", CanWrite: true, Shell: true, Timeout: 3 * time.Second, Approver: approveFunc(func(_ context.Context, r store.NativeApproval) (string, error) {
		asked = append(asked, r.Action)
		return "allow_once", nil
	})}, Request{})
	if err != nil || !slices.Equal(asked, []string{"bash (stage 1 of 3)", "shell (stage 2 of 3)", "shell (stage 3 of 3)"}) {
		t.Fatal(asked, err)
	}
}

// Muse 1.4 offers "abort" as its only refusal; declining must send it and
// report the user's decision rather than an unrecognized failure.
func TestMuseDeclineSendsAbortAndReportsUserDecision(t *testing.T) {
	runner := fixture(t, func(p peer, c process.Command) {
		p.rpc("initialize", dict{})
		p.read()
		p.rpc("session/start", dict{"session": dict{"sessionId": "muse-1"}})
		p.rpc("turn/start", dict{"turnId": "turn-1"})
		p.write(dict{"jsonrpc": "2.0", "id": "server-1", "method": "approval/request", "params": dict{"sessionId": "muse-1", "turnId": "turn-1", "approvalId": "a-1", "currentRequirementId": dict{"approvalId": "a-1", "sourceIndex": 0}, "toolName": "bash", "subject": dict{"kind": "shell", "command": "rm -rf build"}, "availableChoices": []any{dict{"choiceId": "allow_once", "label": "Allow once", "decision": "approved"}, dict{"choiceId": "abort", "label": "Reject", "decision": "abort"}}}})
		p.read()
		if m := p.rpc("approval/decide", dict{"status": "accepted", "terminal": true}); str(m["choiceId"]) != "abort" {
			t.Error(string(raw(m)))
		}
		p.notification("turn/completed", dict{"sessionId": "muse-1", "turnId": "turn-1", "terminal": "aborted"})
	})
	_, err := Muse(t.Context(), runner, Config{Executable: "fixture", CanWrite: true, Shell: true, Timeout: 3 * time.Second, Approver: approveFunc(func(context.Context, store.NativeApproval) (string, error) { return "", nil })}, Request{})
	var denied *store.PermissionDenied
	if !errors.As(err, &denied) || !denied.UserDeclined || denied.Action.Tool != "bash" || denied.Action.Target != "rm -rf build" {
		t.Fatalf("error = %v", err)
	}
}

func TestCodexModelsReadsEveryPageWithoutStartingAThread(t *testing.T) {
	runner := fixture(t, func(p peer, c process.Command) {
		if strings.Join(c.Args, " ") != "app-server" || c.Timeout <= 0 {
			t.Error(c.Args)
		}
		p.rpc("initialize", dict{})
		if str(p.read()["method"]) != "initialized" {
			t.Error("no initialized")
		}
		p.notification("remoteControl/status/changed", dict{"status": "disabled"})
		if cursor := p.rpc("model/list", dict{"data": []any{dict{"id": "gpt-a", "isDefault": true, "description": "First"}, dict{"id": "internal", "hidden": true}}, "nextCursor": "page-2"})["cursor"]; cursor != nil {
			t.Error("first page sent a cursor")
		}
		if cursor := str(p.rpc("model/list", dict{"data": []any{dict{"id": "gpt-b"}}, "nextCursor": nil})["cursor"]); cursor != "page-2" {
			t.Error("second page cursor", cursor)
		}
	})
	models, err := CodexModels(t.Context(), runner, "fixture", "/work")
	want := []Model{{ID: "gpt-a", Description: "First", Default: true}, {ID: "gpt-b"}}
	if err != nil || len(models) != len(want) || models[0] != want[0] || models[1] != want[1] {
		t.Fatal(models, err)
	}
}
