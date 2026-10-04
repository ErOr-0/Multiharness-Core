package cli_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func TestDirectInteractiveSessionsAndExplicitReset(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var sessions []string
	factory := func(cfg config.Config, _ workflow.EventSink) (cli.Runner, error) {
		if cfg.Mode != "direct" {
			t.Fatal("direct must be the default")
		}
		return runFunc(func(_ context.Context, in contract.TaskInput) contract.TaskOutput {
			sessions = append(sessions, in.SessionID)
			return contract.TaskOutput{Status: contract.TaskStatusResponded, Summary: "Native response", Direct: &contract.DirectResponse{Text: "Native response", SessionID: "ses_continued"}, AgentInvocations: 1}
		}), nil
	}
	h := newHandler(t, factory, &stdout, &stderr, t.TempDir(), nil)
	input := &promptLines{lines: []string{"first", "follow up", "/set progress off", "another follow up", "/set implementer-timeout 2h", "after deadline change", "/new", "fresh", "/set implementer-model provider/other", "changed model", "/set implementer-harness codex", "changed provider", "/save", "/quit"}}
	settings := filepath.Join(t.TempDir(), "config.json")
	if code := h.Interactive(t.Context(), input, settings); code != 0 {
		t.Fatalf("%d %s", code, stdout.String())
	}
	if strings.Join(sessions, ",") != ",ses_continued,ses_continued,ses_continued,,," {
		t.Fatalf("crossed conversation boundary: %q", sessions)
	}
	saved, err := config.Load(settings, t.TempDir(), nil, nil)
	if err != nil || saved.SessionID != "" {
		t.Fatalf("persisted conversation: %+v %v", saved, err)
	}
	if strings.Contains(stdout.String(), "approved") || !strings.Contains(stdout.String(), "Native response") {
		t.Fatal(stdout.String())
	}
}

func TestDirectSetupUsesOnlyThreeFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	calls := 0
	factory := func(cfg config.Config, _ workflow.EventSink) (cli.Runner, error) {
		calls++
		if cfg.Implementer.Harness != "codex" || cfg.Implementer.Model != "fixture-model" || cfg.Implementer.Reasoning != "high" {
			t.Fatalf("lost setup: %+v", cfg.Implementer)
		}
		return runFunc(func(context.Context, contract.TaskInput) contract.TaskOutput {
			return contract.TaskOutput{Status: contract.TaskStatusResponded, Summary: "Ready", Direct: &contract.DirectResponse{Text: "Ready"}}
		}), nil
	}
	h := newHandler(t, factory, &stdout, &stderr, t.TempDir(), nil)
	if code := h.Interactive(t.Context(), &promptLines{lines: []string{"/config", "codex", "fixture-model", "high", "task", "/quit"}}, filepath.Join(t.TempDir(), "config.json")); code != 0 || calls != 1 {
		t.Fatalf("%d %d %s", code, calls, stdout.String())
	}
	if !strings.Contains(stdout.String(), "3/3") || strings.Contains(stdout.String(), "/9") {
		t.Fatal(stdout.String())
	}
}

func TestDirectExitStatuses(t *testing.T) {
	for status, code := range map[contract.TaskStatus]int{contract.TaskStatusResponded: 0, contract.TaskStatusNeedsInput: 4, contract.TaskStatusTimedOut: 124} {
		var stdout, stderr bytes.Buffer
		h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
			return runFunc(func(context.Context, contract.TaskInput) contract.TaskOutput {
				r := &contract.DirectResponse{Text: "Agent response", NeedsInput: status == contract.TaskStatusNeedsInput}
				return contract.TaskOutput{Status: status, Summary: r.Text, Direct: r}
			}), nil
		}, &stdout, &stderr, t.TempDir(), nil)
		if got := h.Run(t.Context(), []string{"--quiet", "task"}); got != code {
			t.Fatalf("%s: %d expected %d: %s", status, got, code, stdout.String())
		}
		if out := decodeOutput(t, stdout.Bytes()); out.Status != status {
			t.Fatal(out)
		}
	}
}

func TestInteractivePermissionDenialKeepsNativeConversation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var sessions []string
	h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(_ context.Context, in contract.TaskInput) contract.TaskOutput {
			sessions = append(sessions, in.SessionID)
			if len(sessions) == 1 {
				return contract.TaskOutput{Status: contract.TaskStatusNeedsInput, Summary: "Read blocked: /parent/AGENTS.md", Direct: &contract.DirectResponse{Text: "Checking docs", SessionID: "ses_blocked", NeedsInput: true, Blocked: &contract.BlockedAction{Tool: "read", Target: "/parent/AGENTS.md"}}, AgentInvocations: 1}
			}
			return contract.TaskOutput{Status: contract.TaskStatusResponded, Summary: "Continued inside project", Direct: &contract.DirectResponse{Text: "Continued inside project", SessionID: "ses_blocked"}, AgentInvocations: 1}
		}), nil
	}, &stdout, &stderr, t.TempDir(), nil)
	code := h.Interactive(t.Context(), &promptLines{lines: []string{"check docs", "continue without reading the parent file", "/quit"}}, filepath.Join(t.TempDir(), "config.json"))
	if code != 0 || strings.Join(sessions, ",") != ",ses_blocked" || !strings.Contains(stdout.String(), "/parent/AGENTS.md") || !strings.Contains(stdout.String(), "Continued inside project") {
		t.Fatalf("lost blocked conversation: code=%d sessions=%v output=%s", code, sessions, stdout.String())
	}
}
