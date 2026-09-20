package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func TestContainerModeChangesResetConversationAndPersistRoles(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(settings, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var modes, sessions []string
	h := newHandler(t, func(cfg config.Config, _ workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(_ context.Context, input store.TaskInput) store.TaskOutput {
			modes = append(modes, cfg.Mode)
			sessions = append(sessions, input.SessionID)
			if cfg.Mode == "team" && (cfg.Planner.Model != "fixture/plan" || cfg.Implementer.Model != "fixture/build" || cfg.Reviewer.Model != "fixture/review") {
				t.Fatalf("roles lost: %+v", cfg)
			}
			return store.TaskOutput{Status: store.TaskStatusResponded, Summary: "Done", Direct: &store.DirectResponse{Text: "Done", SessionID: "native-session"}}
		}), nil
	}, &out, &out, root, map[string]string{"MAGENT_WORKSPACE_ROOT": root})
	lines := []string{"", "first task", "/config", "4", "invalid", "2", "/config", "2",
		"opencode", "fixture/plan", "", "opencode", "fixture/build", "", "opencode", "fixture/review", "",
		"team task", "/config", "4", "1", "direct task", "/config", "4", "/cancel", "continue", "/quit"}
	if code := h.Interactive(t.Context(), &promptLines{lines: lines}, settings); code != 0 {
		t.Fatal(code, out.String())
	}
	if strings.Join(modes, ",") != "direct,team,direct,direct" || strings.Join(sessions, ",") != ",,,native-session" {
		t.Fatal(modes, sessions, out.String())
	}
	saved, err := config.Load(settings, root, nil, nil)
	if err != nil || saved.Mode != "direct" || saved.Planner.Model != "fixture/plan" || saved.Implementer.Model != "fixture/build" || saved.Reviewer.Model != "fixture/review" || saved.SessionID != "" {
		t.Fatal(saved, err)
	}
	for _, text := range []string{"Direct - one agent", "Team - separate", "4. Mode", "All available controls", "Menu changes save automatically", "Choose Direct or Team"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("missing %q: %s", text, out.String())
		}
	}
}

func TestContainerModeRejectsIncompatiblePermissionsWithoutLosingSession(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(settings, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	var sessions []string
	h := newHandler(t, func(cfg config.Config, _ workflow.EventSink) (cli.Runner, error) {
		if cfg.Mode != "direct" || cfg.Implementer.Sandbox != "danger-full-access" {
			t.Fatal(cfg)
		}
		return runFunc(func(_ context.Context, input store.TaskInput) store.TaskOutput {
			sessions = append(sessions, input.SessionID)
			return store.TaskOutput{Status: store.TaskStatusResponded, Summary: "Done", Direct: &store.DirectResponse{Text: "Done", SessionID: "kept-session"}}
		}), nil
	}, &out, &out, root, map[string]string{"MAGENT_WORKSPACE_ROOT": root})
	lines := []string{"", "/set implementer-harness codex", "/permissions full", "first", "/config", "4", "2", "continue", "/quit"}
	if code := h.Interactive(t.Context(), &promptLines{lines: lines}, settings); code != 0 {
		t.Fatal(code, out.String())
	}
	if strings.Join(sessions, ",") != ",kept-session" || !strings.Contains(out.String(), "mode not changed") {
		t.Fatal(sessions, out.String())
	}
	saved, err := config.Load(settings, root, nil, nil)
	if err != nil || saved.Mode != "direct" || saved.Implementer.Sandbox != "danger-full-access" {
		t.Fatal(saved, err)
	}
}
