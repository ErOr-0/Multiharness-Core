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

func TestPermissionsMenuSavesAndKeepsBlockedConversation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var policies, sessions []string
	h := newHandler(t, func(cfg config.Config, _ workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(_ context.Context, in store.TaskInput) store.TaskOutput {
			policies = append(policies, string(cfg.Implementer.PermissionPolicy))
			sessions = append(sessions, in.SessionID)
			if cfg.Implementer.PermissionPolicy == "auto_approve" {
				return store.TaskOutput{Status: store.TaskStatusResponded, Summary: "Read allowed", Direct: &store.DirectResponse{Text: "Read allowed", SessionID: "ses_permissions"}, AgentInvocations: 1}
			}
			return store.TaskOutput{Status: store.TaskStatusNeedsInput, Summary: "Read blocked", Direct: &store.DirectResponse{Text: "Read blocked", SessionID: "ses_permissions", NeedsInput: true, Blocked: &store.BlockedAction{Tool: "read", Target: "/parent/AGENTS.md"}}, AgentInvocations: 1}
		}), nil
	}, &stdout, &stderr, t.TempDir(), nil)
	settings := filepath.Join(t.TempDir(), "config.json")
	lines := &promptLines{lines: []string{"read", "/permissions", "bad choice", "2", "retry", "/settings", "/permissions native", "read again", "/quit"}}
	if code := h.Interactive(t.Context(), lines, settings); code != 0 {
		t.Fatalf("%d: %s", code, stdout.String())
	}
	if strings.Join(policies, ",") != "reject_on_prompt,auto_approve,reject_on_prompt" || strings.Join(sessions, ",") != ",ses_permissions,ses_permissions" {
		t.Fatalf("policies=%v sessions=%v", policies, sessions)
	}
	saved, err := config.Load(settings, t.TempDir(), nil, nil)
	if err != nil || saved.Implementer.PermissionPolicy != "reject_on_prompt" || saved.SessionID != "" {
		t.Fatalf("saved=%+v err=%v", saved, err)
	}
	for _, text := range []string{"Use /permissions here", "Choose 1 or 2", "Auto-approve requests (--auto)", "explicit OpenCode deny rules still apply"} {
		if !strings.Contains(stdout.String(), text) {
			t.Fatalf("missing %q: %s", text, stdout.String())
		}
	}
}

func TestPermissionsInvalidAndCancelledChoicesKeepCurrentSettings(t *testing.T) {
	for _, commands := range [][]string{{"/permissions unknown"}, {"/permissions", "/cancel"}, {"/permissions", ""}} {
		var stdout, stderr bytes.Buffer
		h := newHandler(t, func(cfg config.Config, _ workflow.EventSink) (cli.Runner, error) {
			if cfg.Implementer.PermissionPolicy != "reject_on_prompt" {
				t.Fatal("changed permission without a valid choice")
			}
			return runFunc(func(context.Context, store.TaskInput) store.TaskOutput {
				return store.TaskOutput{Status: store.TaskStatusResponded, Summary: "Done", Direct: &store.DirectResponse{Text: "Done"}}
			}), nil
		}, &stdout, &stderr, t.TempDir(), nil)
		if code := h.Interactive(t.Context(), &promptLines{lines: append(commands, "task", "/quit")}, filepath.Join(t.TempDir(), "config.json")); code != 0 {
			t.Fatal(stdout.String())
		}
	}
}

func TestPermissionsRejectOtherProviders(t *testing.T) {
	var stdout, stderr bytes.Buffer
	h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		t.Fatal("permission command must not start a task")
		return nil, nil
	}, &stdout, &stderr, t.TempDir(), nil)
	if code := h.Interactive(t.Context(), &promptLines{lines: []string{"/set implementer-harness codex", "/permissions auto", "/quit"}}, filepath.Join(t.TempDir(), "config.json")); code != 0 || !strings.Contains(stdout.String(), "currently configures OpenCode") {
		t.Fatal(stdout.String())
	}
}
