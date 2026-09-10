package schemaexec

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

type claudeRunnerFunc func(context.Context, process.Command) (process.Result, error)

func (f claudeRunnerFunc) Run(ctx context.Context, c process.Command) (process.Result, error) {
	return f(ctx, c)
}

const claudeAnswer = `{"type":"result","subtype":"success","is_error":false,"structured_output":{"schema_version":"2","action":"answer","answer":"Done","summary":"answer","steps":[],"acceptance_criteria":[]}}`

func TestClaudeRejectsFailedOrAmbiguousResponses(t *testing.T) {
	for _, output := range []string{
		strings.Replace(claudeAnswer, `"is_error":false`, `"is_error":true`, 1),
		strings.Replace(claudeAnswer, `"is_error":false`, `"Is_Error":false`, 1),
		strings.Replace(claudeAnswer, `"is_error":false`, `"is_error":true,"is_error":false`, 1),
		strings.Replace(claudeAnswer, `"subtype":"success"`, `"subtype":"error_max_turns"`, 1),
		strings.Replace(claudeAnswer, `"is_error":false,`, "", 1),
		strings.Replace(claudeAnswer, `"is_error":false`, `"is_error":null`, 1),
		strings.Replace(claudeAnswer, `"is_error":false`, `"is_error":false,"permission_denials":[{"tool_name":"Write"}]`, 1),
		strings.Replace(claudeAnswer, `"structured_output"`, `"result"`, 1),
		claudeAnswer + claudeAnswer,
	} {
		a, err := NewClaude(claudeRunnerFunc(func(context.Context, process.Command) (process.Result, error) {
			return process.Result{Stdout: output}, nil
		}), ClaudeConfig{Executable: "claude", Model: "sonnet", Timeout: time.Minute})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.Plan(t.Context(), store.TaskInput{Task: "explain", WorkingDir: t.TempDir()}); err == nil {
			t.Fatalf("accepted invalid completion: %s", output)
		}
	}
}
func TestClaudeContextPermissionsAndOutputLimits(t *testing.T) {
	input := store.TaskInput{Task: "private task", WorkingDir: t.TempDir()}
	ctx := context.WithValue(t.Context(), struct{}{}, "context")
	a, err := NewClaude(claudeRunnerFunc(func(call context.Context, c process.Command) (process.Result, error) {
		prompt, _ := io.ReadAll(c.Stdin)
		if call.Value(struct{}{}) != "context" || c.Dir != input.WorkingDir || c.Timeout != time.Minute || !strings.Contains(string(prompt), input.Task) || strings.Contains(strings.Join(c.Args, " "), input.Task) {
			t.Fatal("lost context or exposed prompt")
		}
		args := strings.Join(c.Args, " ")
		for _, want := range []string{"--permission-mode dontAsk", "--tools Read,Glob,Grep", "--allowedTools Read,Glob,Grep", "--no-session-persistence", "--strict-mcp-config", "disableAllHooks"} {
			if !strings.Contains(args, want) {
				t.Fatal("missing permission boundary", want)
			}
		}
		if strings.Contains(args, "Edit") || strings.Contains(args, "Bash") || strings.Contains(args, "--resume") {
			t.Fatal("unsafe read-only tools/session")
		}
		return process.Result{Stdout: claudeAnswer}, nil
	}), ClaudeConfig{Executable: "claude", Model: "sonnet", Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := a.Plan(ctx, input); err != nil || result.Answer != "Done" {
		t.Fatal(result, err)
	}
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded} {
		a.runner = claudeRunnerFunc(func(context.Context, process.Command) (process.Result, error) { return process.Result{}, failure })
		if _, err := a.Plan(t.Context(), input); !errors.Is(err, failure) {
			t.Fatal("lost cancellation", err)
		}
	}
	a.runner = claudeRunnerFunc(func(context.Context, process.Command) (process.Result, error) {
		return process.Result{Stdout: claudeAnswer, StdoutTruncated: true}, nil
	})
	if _, err := a.Plan(t.Context(), input); err == nil {
		t.Fatal("approved truncated output")
	}
}
