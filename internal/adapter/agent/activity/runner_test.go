package activity

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
)

type runFunc func(context.Context, process.Command) (process.Result, error)

func (f runFunc) Run(ctx context.Context, command process.Command) (process.Result, error) {
	return f(ctx, command)
}

func TestStreamingActivityIsBoundedRedactedAndDoesNotAlterExecution(t *testing.T) {
	for _, agent := range []Agent{Codex, OpenCode} {
		t.Run(string(agent), func(t *testing.T) {
			args := []string{"exec", "--json"}
			line := `{"type":"item.started","item":{"type":"command_execution","command":"SECRET\u001b[2J","status":"in_progress"}}`
			want := CommandRunning
			if agent == OpenCode {
				args = []string{"run", "--format", "json"}
				line = `{"type":"tool_use","sessionID":"SECRET","part":{"type":"tool","tool":"SECRET","state":{"status":"completed"}}}`
				want = ToolFinished
			}
			var captured bytes.Buffer
			var events []Event
			sentinel := errors.New("execution failed")
			payload := line + "\n" + strings.Repeat("x", maxLineBytes+1) + "\n" + "{malformed}\n" + line
			calls := 0
			runner := Runner{Agent: agent, Observe: func(event Event) { events = append(events, event) }, Runner: runFunc(func(ctx context.Context, command process.Command) (process.Result, error) {
				calls++
				if ctx != t.Context() || command.Name != "fixture" || !reflect.DeepEqual(command.Args, args) || command.EnvOverrides["SECRET"] != "unchanged" {
					t.Fatal("execution settings changed")
				}
				if len(events) != 1 || events[0].Kind != Starting {
					t.Fatal("missing invocation start")
				}
				for offset := 0; offset < len(payload); offset += 13 {
					if _, err := io.WriteString(command.Stdout, payload[offset:min(offset+13, len(payload))]); err != nil {
						t.Fatal(err)
					}
				}
				if len(events) < 2 {
					t.Fatal("events were withheld until process exit")
				}
				return process.Result{ExitCode: 7, Stderr: "private diagnostics"}, sentinel
			})}
			result, err := runner.Run(t.Context(), process.Command{Name: "fixture", Args: args, Stdout: &captured, EnvOverrides: map[string]string{"SECRET": "unchanged"}})
			if calls != 1 || !errors.Is(err, sentinel) || result.ExitCode != 7 || result.Stderr != "private diagnostics" || captured.String() != payload {
				t.Fatal("telemetry changed result, bytes or call count")
			}
			if !reflect.DeepEqual(events, []Event{{Agent: agent, Kind: Starting}, {Agent: agent, Kind: want}, {Agent: agent, Kind: want}}) {
				t.Fatalf("events=%v", events)
			}
		})
	}
}

func TestActivityMetadataMappingAndUnknownEvents(t *testing.T) {
	for _, test := range []struct {
		agent Agent
		line  string
		want  Kind
	}{
		{Codex, `{"type":"turn.started"}`, TurnStarted},
		{Codex, `{"type":"turn.completed"}`, StepFinished},
		{Codex, `{"type":"item.completed","item":{"type":"command_execution","status":"completed"}}`, CommandFinished},
		{Codex, `{"type":"item.completed","item":{"type":"command_execution","status":"failed"}}`, ToolFailed},
		{Codex, `{"type":"item.completed","item":{"type":"file_change"}}`, FilesChanged},
		{Codex, `{"type":"item.completed","item":{"type":"file_change","status":"failed"}}`, ToolFailed},
		{Codex, `{"type":"item.started","item":{"type":"mcp_tool_call"}}`, ToolRunning},
		{Codex, `{"type":"item.completed","item":{"type":"web_search"}}`, ToolFinished},
		{Codex, `{"type":"item.completed","item":{"type":"mcp_tool_call","status":"failed"}}`, ToolFailed},
		{Codex, `{"type":"item.completed","item":{"type":"agent_message","text":"SECRET"}}`, ResponseReceived},
		{Codex, `{"type":"item.completed","item":{"type":"reasoning","text":"SECRET"}}`, ""},
		{Codex, `{"type":"error","message":"SECRET"}`, ""},
		{Codex, `{"type":"unknown","item":{"type":"command_execution"}}`, ""},
		{OpenCode, `{"type":"step_start","part":{"type":"step-start"}}`, TurnStarted},
		{OpenCode, `{"type":"step_finish","part":{"type":"step-finish"}}`, StepFinished},
		{OpenCode, `{"type":"text","part":{"type":"text","text":"SECRET"}}`, ResponseReceived},
		{OpenCode, `{"type":"tool_use","part":{"type":"tool","state":{"status":"error"}}}`, ToolFailed},
		{OpenCode, `{"type":"tool_use","part":{"type":"unknown","state":{"status":"completed"}}}`, ""},
		{OpenCode, `{"type":"step_finish","part":{"type":"unknown"}}`, ""},
		{OpenCode, `{"type":"reasoning","part":{"text":"SECRET"}}`, ""},
		{OpenCode, `{`, ""},
	} {
		if got := decode(test.agent, []byte(test.line)); got != test.want {
			t.Errorf("decode(%s, %s)=%q; want %q", test.agent, test.line, got, test.want)
		}
	}
	if (Event{Agent: "SECRET", Kind: Starting}).Valid() || (Event{Agent: Codex, Kind: "SECRET"}).Valid() {
		t.Fatal("untrusted display metadata accepted")
	}
}

func TestActivityDoesNotProbeRetryOrSwallowCancellation(t *testing.T) {
	for _, args := range [][]string{nil, {"--version"}, {"exec", "--help"}, {"exec", "--json"}} {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		calls := 0
		r := Runner{Agent: Codex, Observe: func(Event) { t.Fatal("unexpected activity") }, Runner: runFunc(func(got context.Context, command process.Command) (process.Result, error) {
			calls++
			if got != ctx || command.Stdout != nil {
				t.Fatal("probe or cancelled execution decorated")
			}
			return process.Result{}, ctx.Err()
		})}
		if _, err := r.Run(ctx, process.Command{Args: args}); !errors.Is(err, context.Canceled) || calls != 1 {
			t.Fatal("lost cancellation or replayed command")
		}
	}
}
