package cli_test

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

type promptLines struct{ lines []string }

func (p *promptLines) ReadLine(ctx context.Context, _ int) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(p.lines) == 0 {
		return "", io.EOF
	}
	line := p.lines[0]
	p.lines = p.lines[1:]
	return line, nil
}

func TestInteractiveSettingsAndIndependentTasks(t *testing.T) {
	var stdout, stderr bytes.Buffer
	base := t.TempDir()
	settings := filepath.Join(t.TempDir(), "magent", "config.json")
	calls := 0
	factory := func(cfg config.Config, events workflow.EventSink) (cli.Runner, error) {
		if cfg.Implementer.Model != "fixture/model" || cfg.MaxRepairAttempts != 2 {
			t.Fatalf("settings were lost or invalid update accepted: %+v", cfg)
		}
		return runFunc(func(ctx context.Context, input store.TaskInput) store.TaskOutput {
			calls++
			want := "first task"
			if calls == 2 {
				want = "second task"
			}
			if input.Task != want || ctx.Err() != nil {
				t.Fatalf("independent task handoff: %+v", input)
			}
			result := exampleOutput(store.TaskStatusAnswered)
			result.Plan.Answer = "Readable answer\x1b[2J\u202e"
			result.Summary = result.Plan.Answer
			return result
		}), nil
	}
	h := newHandler(t, factory, &stdout, &stderr, base, nil)
	input := &promptLines{lines: []string{
		"/config", "", "", "", "fixture/model", "",
		"/set max-repair-attempts 2", "/set max-repair-attempts -1", "/save",
		"first task", "second task", "/quit",
	}}
	if code := h.Interactive(t.Context(), input, settings); code != 0 || calls != 2 {
		t.Fatalf("code=%d calls=%d output=%s", code, calls, stdout.String())
	}
	if strings.Contains(stdout.String(), "\x1b") || strings.Contains(stdout.String(), "\u202e") || !strings.Contains(stdout.String(), "Readable answer") {
		t.Fatalf("unsafe or missing answer: %q", stdout.String())
	}
	// Personal defaults follow the new invocation directory and never resume
	// a previous task, while retaining selected agent models and repair limits.
	otherBase := t.TempDir()
	loaded, err := config.Load(settings, otherBase, nil, nil)
	if err != nil || loaded.WorkingDir != otherBase || loaded.SessionID != "" || loaded.Implementer.Model != "fixture/model" {
		t.Fatalf("saved defaults: %+v, %v", loaded, err)
	}
}

func TestInteractiveCancellationAndOutputFailureNeverStartMoreTasks(t *testing.T) {
	for _, brokenOutput := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		calls := 0
		var stdout, stderr bytes.Buffer
		var output io.Writer = &stdout
		if brokenOutput {
			output = interactiveBrokenWriter{}
		}
		factory := func(config.Config, workflow.EventSink) (cli.Runner, error) {
			return runFunc(func(callCtx context.Context, _ store.TaskInput) store.TaskOutput {
				calls++
				cancel()
				<-callCtx.Done()
				return exampleOutput(store.TaskStatusCancelled)
			}), nil
		}
		h := newHandler(t, factory, output, &stderr, t.TempDir(), nil)
		code := h.Interactive(ctx, &promptLines{lines: []string{"task", "must not run"}}, filepath.Join(t.TempDir(), "config.json"))
		cancel()
		if brokenOutput && (calls != 0 || code != cli.ExitFailed) {
			t.Fatalf("output failure started work: code=%d calls=%d", code, calls)
		}
		if !brokenOutput && (calls != 1 || code != cli.ExitCancelled) {
			t.Fatalf("cancellation started another task: code=%d calls=%d", code, calls)
		}
	}
}

type interactiveBrokenWriter struct{}

func (interactiveBrokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestInteractiveCodexImplementationSelectionAndSave(t *testing.T) {
	var stdout, stderr bytes.Buffer
	calls := 0
	file := filepath.Join(t.TempDir(), "config.json")
	factory := func(cfg config.Config, _ workflow.EventSink) (cli.Runner, error) {
		if cfg.Planner.Model != "gpt-6-astra" || cfg.Implementer.Harness != "codex" || cfg.Implementer.Executable != "codex" || cfg.Implementer.Model != "gpt-5.6-luna" || cfg.Implementer.Sandbox != "workspace-write" {
			t.Fatalf("role selection lost: %+v", cfg.Implementer)
		}
		return runFunc(func(context.Context, store.TaskInput) store.TaskOutput {
			calls++
			return exampleOutput(store.TaskStatusAnswered)
		}), nil
	}
	h := newHandler(t, factory, &stdout, &stderr, t.TempDir(), nil)
	lines := []string{"/config", "codex", "gpt-6-astra", "codex", "gpt-5.6-luna", "", "/save", "/settings", "explain", "/quit"}
	if code := h.Interactive(t.Context(), &promptLines{lines: lines}, file); code != 0 || calls != 1 {
		t.Fatalf("code=%d calls=%d output=%s", code, calls, stdout.String())
	}
	loaded, err := config.Load(file, t.TempDir(), nil, nil)
	if err != nil || loaded.Implementer.Harness != "codex" || loaded.Implementer.Model != "gpt-5.6-luna" {
		t.Fatalf("saved Codex selection lost: %v", err)
	}
	if !strings.Contains(stdout.String(), "BUILD    Codex") {
		t.Fatal("settings still labels the implementer OpenCode")
	}
}

func TestInteractiveConfigurationRecoversWithoutGuessingActions(t *testing.T) {
	var stdout, stderr bytes.Buffer
	calls := 0
	factory := func(cfg config.Config, _ workflow.EventSink) (cli.Runner, error) {
		if cfg.Planner.Harness != "opencode" || cfg.Planner.Model != "Provider/Planner" || cfg.Implementer.Model != "Provider/ExactCase" || cfg.Reviewer.Model != "ReviewerCase" || cfg.MaxRepairAttempts != 3 {
			t.Fatalf("configuration answers lost or IDs rewritten: %+v", cfg)
		}
		if cfg.Implementer.PermissionPolicy != "reject_on_prompt" {
			t.Fatal("invalid permission input broadened access")
		}
		return runFunc(func(_ context.Context, in store.TaskInput) store.TaskOutput {
			calls++
			if in.Task != "Explain teh repo; keep `ExactCase` and $HOME literal" {
				t.Fatalf("task text rewritten: %q", in.Task)
			}
			return exampleOutput(store.TaskStatusAnswered)
		}), nil
	}
	h := newHandler(t, factory, &stdout, &stderr, t.TempDir(), nil)
	lines := []string{
		"/confg", // Suggest, without opening a wizard or launching an agent.
		"/CONFIG", "opencod", " OPENCODE ", "Provider/Planner",
		"opencode", "wrong model", "Provider/Original", "ReviewerCase",
		"/SET\t--implementer_model = “Provider/ExactCase”",
		"/set max_repair_attempts=003",
		"/set implementer-permission-policy auto_aprove",
		"/set implementer-model", // Missing value must not clear the model.
		"/set implementer-model bad\x00input",
		"/set implementer-model \"unfinished",
		"/load \"\"",
		"/set implementer-modle Another/Model", // Suggest, never apply.
		"/config", "codex", "/cancel",          // Discard a partial reconfiguration.
		"Explain teh repo; keep `ExactCase` and $HOME literal", "/quit",
	}
	if code := h.Interactive(t.Context(), &promptLines{lines: lines}, filepath.Join(t.TempDir(), "config.json")); code != 0 || calls != 1 {
		t.Fatalf("code=%d calls=%d output=%s", code, calls, stdout.String())
	}
	for _, want := range []string{"Did you mean /config?", "earlier answers are kept", "Did you mean implementer-model?", "use \"\" to clear", "Setup cancelled"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("missing recovery guidance %q: %s", want, stdout.String())
		}
	}
}

func TestInteractiveColorsRespectUserPreferences(t *testing.T) {
	for _, test := range []struct {
		name      string
		env       map[string]string
		wantColor bool
	}{
		{"enabled", map[string]string{"MULTIHARNESS_COLOR": "always"}, true},
		{"disabled", map[string]string{"MULTIHARNESS_COLOR": "never"}, false},
		{"no color", map[string]string{"MULTIHARNESS_COLOR": "always", "NO_COLOR": "1"}, false},
		{"dumb terminal", map[string]string{"MULTIHARNESS_COLOR": "always", "TERM": "dumb"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
				t.Fatal("opening/configuring the UI must not call a provider")
				return nil, nil
			}, &stdout, &stderr, t.TempDir(), test.env)
			if code := h.Interactive(t.Context(), &promptLines{lines: []string{"/quit"}}, filepath.Join(t.TempDir(), "config.json")); code != 0 {
				t.Fatalf("exit %d", code)
			}
			if strings.Contains(stdout.String(), "\x1b[") != test.wantColor {
				t.Fatalf("colour preference ignored: %q", stdout.String())
			}
		})
	}
}

func TestContainerAccountLoginUsesInjectedCallbackWithoutStartingTask(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		t.Fatal("login started a task")
		return nil, nil
	}, &out, &out, root, map[string]string{"MAGENT_WORKSPACE_ROOT": root})
	var providers []string
	ctx := context.WithValue(t.Context(), struct{}{}, "login context")
	h.SetAccountLogin(func(received context.Context, provider string) error {
		if received != ctx {
			t.Fatal("lost login context")
		}
		providers = append(providers, provider)
		return nil
	})
	lines := []string{"", "/login unexpected", "/login codex extra", "/login codex", "/login opencode", "/quit"}
	if code := h.Interactive(ctx, &promptLines{lines: lines}, filepath.Join(t.TempDir(), "config.json")); code != 0 || strings.Join(providers, ",") != "codex,opencode" {
		t.Fatal(code, providers, out.String())
	}
}
