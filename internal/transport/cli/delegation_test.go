package cli_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
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
		return runFunc(func(_ context.Context, in store.TaskInput) store.TaskOutput {
			sessions = append(sessions, in.SessionID)
			return store.TaskOutput{Status: store.TaskStatusResponded, Summary: "Native response", Direct: &store.DirectResponse{Text: "Native response", SessionID: "ses_continued"}, AgentInvocations: 1}
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
		if cfg.Implementer.Model != "provider/model" || cfg.Implementer.Variant != "high" {
			t.Fatalf("lost setup: %+v", cfg.Implementer)
		}
		return runFunc(func(context.Context, store.TaskInput) store.TaskOutput {
			return store.TaskOutput{Status: store.TaskStatusResponded, Summary: "Ready", Direct: &store.DirectResponse{Text: "Ready"}}
		}), nil
	}
	h := newHandler(t, factory, &stdout, &stderr, t.TempDir(), nil)
	if code := h.Interactive(t.Context(), &promptLines{lines: []string{"/config", "opencode", "provider/model", "high", "task", "/quit"}}, filepath.Join(t.TempDir(), "config.json")); code != 0 || calls != 1 {
		t.Fatalf("%d %d %s", code, calls, stdout.String())
	}
	if !strings.Contains(stdout.String(), "3/3") || strings.Contains(stdout.String(), "/9") {
		t.Fatal(stdout.String())
	}
}

func TestDirectExitStatuses(t *testing.T) {
	for status, code := range map[store.TaskStatus]int{store.TaskStatusResponded: 0, store.TaskStatusNeedsInput: 4, store.TaskStatusTimedOut: 124} {
		var stdout, stderr bytes.Buffer
		h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
			return runFunc(func(context.Context, store.TaskInput) store.TaskOutput {
				r := &store.DirectResponse{Text: "Agent response", NeedsInput: status == store.TaskStatusNeedsInput}
				return store.TaskOutput{Status: status, Summary: r.Text, Direct: r}
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
