package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func TestConfigurationOffersOnlyModelsTheSelectedCLIReports(t *testing.T) {
	var stdout, stderr bytes.Buffer
	h := newTeamHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		t.Fatal("configuration started a task")
		return nil, nil
	}, &stdout, &stderr, t.TempDir(), nil)
	listed := map[string]int{}
	h.SetModelCatalog(func(_ context.Context, r account.Request) ([]account.Model, error) {
		listed[r.Harness]++
		switch r.Harness {
		case "claude":
			return []account.Model{{ID: "sonnet", Default: true}, {ID: "opus", Description: "Most capable\x1b[2J"}}, nil
		case "codex":
			return []account.Model{{ID: "gpt-a", Default: true}, {ID: "gpt-b"}}, nil
		}
		return nil, errors.New("signed out")
	})
	settings := filepath.Join(t.TempDir(), "config.json")
	lines := []string{
		"/config",
		"claude", "2", "", // Planner: a number selects from the list.
		"codex", "made-up", "9", "GPT-B", "", // Implementer: unknown names and numbers are refused.
		"muse", "muse-guess", "", "", // Reviewer: nothing listed, so only the current value is kept.
		"/set implementer-model nope", "/set implementer-model 1", "/set implementer-model gpt-a", "/save", "/quit",
	}
	if code := h.Interactive(t.Context(), &promptLines{lines: lines}, settings); code != 0 {
		t.Fatalf("exit %d: %s", code, stdout.String())
	}
	data, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	var saved config.Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Planner.Model != "opus" || saved.Implementer.Model != "gpt-a" || saved.Reviewer.Model != config.DefaultPlanner("muse").Model {
		t.Fatalf("models: planner=%q implementer=%q reviewer=%q", saved.Planner.Model, saved.Implementer.Model, saved.Reviewer.Model)
	}
	out := stdout.String()
	for _, want := range []string{
		"1. sonnet (current, CLI default)", "2. opus - Most capable",
		`"made-up" is not an available Codex model`, "choose a number from 1 to 2",
		"Muse lists models after sign-in", `Muse Code did not confirm "muse-guess"`,
		`"nope" is not an available Codex model`, "use a model name with /set",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[2J") {
		t.Fatal("provider description reached the terminal as a control sequence")
	}
	if listed["codex"] != 4 || listed["claude"] != 1 || listed["muse"] != 1 {
		t.Fatalf("catalog queries: %v", listed)
	}
}

func TestConfigurationRequiresAChoiceWhenTheCurrentModelIsUnavailable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	h := newTeamHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		t.Fatal("configuration started a task")
		return nil, nil
	}, &stdout, &stderr, t.TempDir(), map[string]string{"MULTIHARNESS_PLANNER_HARNESS": "claude", "MULTIHARNESS_PLANNER_MODEL": "sonnet-5.5"})
	h.SetModelCatalog(func(context.Context, account.Request) ([]account.Model, error) {
		return []account.Model{{ID: "sonnet"}, {ID: "opus"}}, nil
	})
	settings := filepath.Join(t.TempDir(), "config.json")
	lines := []string{"/config", "", "", "sonnet", "", "/cancel", "/quit"}
	if code := h.Interactive(t.Context(), &promptLines{lines: lines}, settings); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if out := stdout.String(); !strings.Contains(out, "The current model is not available here") || !strings.Contains(out, `"sonnet-5.5" is not an available Claude model`) {
		t.Fatalf("unavailable current model was kept silently:\n%s", out)
	}
}
