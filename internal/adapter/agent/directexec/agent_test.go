package directexec

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

type runnerFunc func(context.Context, process.Command) (process.Result, error)

func (f runnerFunc) Run(ctx context.Context, c process.Command) (process.Result, error) {
	return f(ctx, c)
}

var fixtures = map[string]string{
	"codex": `{"type":"thread.started","thread_id":"ses_test"}
{"type":"item.completed","item":{"type":"agent_message","text":"Done, with tests."}}
{"type":"turn.completed"}
`,
	"opencode": `{"type":"step_start","sessionID":"ses_test","part":{"type":"step-start"}}
{"type":"text","sessionID":"ses_test","part":{"type":"text","text":"Done, with tests."}}
{"type":"step_finish","sessionID":"ses_test","part":{"type":"step-finish","reason":"stop"}}
`,
	"claude": `{"type":"system","subtype":"init","session_id":"ses_test"}
{"type":"assistant","session_id":"ses_test","message":{"content":[{"type":"text","text":"Working."}]}}
{"type":"result","subtype":"success","is_error":false,"session_id":"ses_test","result":"Done, with tests."}
`,
}

func TestNativeProtocolsAndResumeCommands(t *testing.T) {
	for harness, events := range fixtures {
		for _, session := range []string{"", "ses_test"} {
			t.Run(harness+session, func(t *testing.T) {
				calls := 0
				r := runnerFunc(func(_ context.Context, c process.Command) (process.Result, error) {
					calls++
					prompt, _ := io.ReadAll(c.Stdin)
					if string(prompt) != "Build an example; $(not a shell command)" || c.Dir != "/workspace" || c.Timeout != 0 {
						t.Fatalf("incorrect delegation: %+v %q", c, prompt)
					}
					joined := strings.Join(c.Args, " ")
					for _, forbidden := range []string{"--output-schema", "--json-schema", "--ephemeral", "--no-session-persistence", "--dangerously-bypass", "--auto"} {
						if strings.Contains(joined, forbidden) {
							t.Fatalf("unexpected flag: %s", joined)
						}
					}
					if session != "" && !strings.Contains(joined, session) {
						t.Fatal("session not resumed")
					}
					if harness == "codex" && (!strings.Contains(joined, `sandbox_mode="workspace-write"`) || !strings.Contains(joined, `approval_policy="never"`)) {
						t.Fatal("permissions not preserved")
					}
					// Fragmented writes exercise event framing independently of OS buffering.
					for _, b := range []byte(events) {
						if _, err := c.Stdout.Write([]byte{b}); err != nil {
							t.Fatal(err)
						}
					}
					return process.Result{ExitCode: 0}, nil
				})
				a, _ := New(r, Config{Harness: harness, Executable: harness, Model: "configured-model", Reasoning: "high", PermissionPolicy: "reject_on_prompt"})
				out, err := a.Execute(t.Context(), store.TaskInput{Task: "Build an example; $(not a shell command)", WorkingDir: "/workspace", SessionID: session})
				if err != nil || out.Text != "Done, with tests." || out.SessionID != "ses_test" || calls != 1 {
					t.Fatalf("response=%+v err=%v calls=%d", out, err, calls)
				}
			})
		}
	}
}

func TestPartialOutputSurvivesTimeout(t *testing.T) {
	a, _ := New(runnerFunc(func(_ context.Context, c process.Command) (process.Result, error) {
		_, _ = io.WriteString(c.Stdout, `{"type":"thread.started","thread_id":"ses_partial"}`+"\n"+`{"type":"item.completed","item":{"type":"agent_message","text":"Edited one file."}}`+"\n")
		return process.Result{ExitCode: -1}, context.DeadlineExceeded
	}), Config{Harness: "codex", Executable: "codex"})
	out, err := a.Execute(t.Context(), store.TaskInput{Task: "edit", WorkingDir: "/workspace"})
	if !errors.Is(err, context.DeadlineExceeded) || out.Text != "Edited one file." || out.SessionID != "ses_partial" {
		t.Fatalf("%+v %v", out, err)
	}
}

func TestMalformedIncompleteAndCrossSessionStreamsFail(t *testing.T) {
	for _, data := range []string{"not json\n", `{"type":"turn.started"}` + "\n", fixtures["codex"] + `{"type":"thread.started","thread_id":"another"}` + "\n", strings.Repeat("x", maxEventBytes+1)} {
		s := newStream("codex", "")
		_, _ = s.Write([]byte(data))
		if _, err := s.finish(); err == nil {
			t.Fatal("accepted invalid stream")
		}
	}
}

func TestClaudePermissionDenialIsNotSuccessfulCompletion(t *testing.T) {
	s := newStream("claude", "")
	_, _ = s.Write([]byte(`{"type":"result","subtype":"success","is_error":false,"session_id":"ses_test","result":"Need Bash permission","permission_denials":[{"tool_name":"Bash"}]}`))
	out, err := s.finish()
	if err != nil || !out.NeedsInput {
		t.Fatalf("%+v %v", out, err)
	}
}
