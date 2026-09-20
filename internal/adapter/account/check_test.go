package account

import (
	"context"
	"errors"
	"multiharness-core/internal/adapter/process"
	"strings"
	"testing"
)

type fakeRunner func(context.Context, process.Command) (process.Result, error)

func (f fakeRunner) Run(ctx context.Context, c process.Command) (process.Result, error) {
	return f(ctx, c)
}
func TestAccountChecks(t *testing.T) {
	for _, tc := range []struct {
		name, harness, model, stdout, stderr string
		exit                                 int
		ready                                bool
	}{
		{"codex signed in", "codex", "model", "", "Logged in using ChatGPT", 0, true},
		{"codex missing", "codex", "model", "", "Not logged in", 1, false},
		{"codex unexpected success", "codex", "model", "PRIVATE_TOKEN", "", 0, false},
		{"claude signed in", "claude", "sonnet", `{"loggedIn":true,"email":"PRIVATE_TOKEN"}`, "", 0, true},
		{"claude signed out", "claude", "sonnet", `{"loggedIn":false}`, "", 0, false},
		{"claude malformed", "claude", "sonnet", `PRIVATE_TOKEN`, "", 0, false},
		{"opencode provider matches", "opencode", "openai/model", "opencode/free\nopenai/model\n", "", 0, true},
		{"opencode wrong account", "opencode", "anthropic/model", "openai/model\n", "", 0, false},
		{"opencode no selected model", "opencode", "", "openai/model\n", "", 0, false},
		{"opencode exact model required", "opencode", "openai/model", "openai/model-other\n", "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := fakeRunner(func(ctx context.Context, c process.Command) (process.Result, error) {
				if c.Name != "/chosen/cli" || c.Dir != "/chosen/workspace" || c.Stdout != nil || c.Stderr != nil || c.Stdin != nil || c.Timeout <= 0 || c.OutputLimit <= 0 {
					t.Fatal("unsafe or wrong account check", c)
				}
				return process.Result{Stdout: tc.stdout, Stderr: tc.stderr, ExitCode: tc.exit}, nil
			})
			s := Check(t.Context(), runner, Request{Harness: tc.harness, Executable: "/chosen/cli", Directory: "/chosen/workspace", Model: tc.model})
			if s.Ready != tc.ready || strings.Contains(s.Detail, "PRIVATE_TOKEN") {
				t.Fatal(s)
			}
		})
	}
	for _, result := range []process.Result{{Stdout: "Logged in using ChatGPT", StdoutTruncated: true}, {Stderr: "Logged in using ChatGPT", StderrTruncated: true}} {
		if Check(t.Context(), fakeRunner(func(context.Context, process.Command) (process.Result, error) { return result, nil }), Request{Harness: "codex"}).Ready {
			t.Fatal("truncated status accepted")
		}
	}
	if Check(t.Context(), fakeRunner(func(context.Context, process.Command) (process.Result, error) {
		return process.Result{}, errors.New("PRIVATE_TOKEN")
	}), Request{Harness: "codex"}).Ready {
		t.Fatal("process failure accepted")
	}
}
