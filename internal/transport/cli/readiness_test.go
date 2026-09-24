package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type setupLines struct{ lines []string }

func (p *setupLines) ReadLine(context.Context, int) (string, error) {
	s := p.lines[0]
	p.lines = p.lines[1:]
	return s, nil
}

func TestReadinessChecksAllSelectedRolesAndBlocksTask(t *testing.T) {
	cfg := config.Defaults()
	cfg.Mode = "team"
	cfg.Fallback.Mode = "disabled"
	cfg.Planner = config.DefaultPlanner("codex")
	cfg.Implementer = config.DefaultImplementer("opencode")
	cfg.Implementer.Model = "anthropic/model"
	cfg.Reviewer = config.DefaultPlanner("claude")
	cfg.Decision.Enabled = true
	filename := filepath.Join(t.TempDir(), "config.json")
	if err := saveInteractiveConfig(filename, cfg); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	h, _ := NewHandler(func(config.Config, workflow.EventSink) (Runner, error) {
		t.Fatal("unready task started")
		return nil, nil
	}, &out, &out, t.TempDir(), nil)
	seen := map[string]bool{}
	jev := 0
	h.SetReadiness(func(ctx context.Context, r account.Request) account.Status {
		seen[r.Harness] = true
		return account.Status{Ready: r.Harness != "claude", Detail: "fixture status"}
	}, func(context.Context, config.Config, bool) account.Status {
		jev++
		return account.Status{Ready: true, Detail: "key checked"}
	})
	code := h.Interactive(t.Context(), &setupLines{[]string{"/configuration", "review this project", "/quit"}}, filename)
	if code != 0 || len(seen) != 3 || jev != 3 || !strings.Contains(out.String(), "Tasks are blocked") || !strings.Contains(out.String(), "Reviewer      Claude") || !strings.Contains(out.String(), "! NEEDS SETUP") {
		t.Fatal(code, seen, jev, out.String())
	}
}

func TestUnreadyTaskStillShowsSetupDetailsAndDoesNotRun(t *testing.T) {
	var out bytes.Buffer
	h, err := NewHandler(func(config.Config, workflow.EventSink) (Runner, error) {
		t.Fatal("unready task started")
		return nil, nil
	}, &out, &out, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	h.SetReadiness(func(context.Context, account.Request) account.Status {
		return account.Status{Detail: "Sign in with /login codex"}
	}, nil)
	code := h.Interactive(t.Context(), &setupLines{[]string{"answer my question", "/quit"}}, filepath.Join(t.TempDir(), "config.json"))
	if code != 0 || strings.Count(out.String(), "WORKFLOW READINESS") != 2 || !strings.Contains(out.String(), "Tasks are blocked") {
		t.Fatal(code, out.String())
	}
}
func TestDirectReadinessIgnoresUnusedAgentsAndJev(t *testing.T) {
	cfg := config.Defaults()
	cfg.Mode = "direct"
	cfg.Decision.Enabled = true
	cfg.Implementer = config.DefaultImplementer("codex")
	var out bytes.Buffer
	h := &Handler{stdout: &out}
	h.SetReadiness(func(ctx context.Context, r account.Request) account.Status {
		if r.Harness != "codex" {
			t.Fatal(r)
		}
		return account.Status{Ready: true}
	}, func(context.Context, config.Config, bool) account.Status {
		t.Fatal("unused Jev checked")
		return account.Status{}
	})
	ready, err := h.readiness(t.Context(), cfg, &interactiveView{writer: &out}, false)
	if !ready || err != nil || !strings.Contains(out.String(), "NOT REQUIRED") {
		t.Fatal(ready, err, out.String())
	}
}
func TestSelectedAccountsAreRechecked(t *testing.T) {
	cfg := config.Defaults()
	cfg.Mode = "team"
	var out bytes.Buffer
	h := &Handler{stdout: &out}
	loggedIn := false
	calls := 0
	h.SetReadiness(func(context.Context, account.Request) account.Status { calls++; return account.Status{Ready: loggedIn} }, nil)
	if ready, _ := h.readiness(t.Context(), cfg, &interactiveView{writer: &out}, false); ready {
		t.Fatal("unsigned accounts passed")
	}
	loggedIn = true
	if ready, _ := h.readiness(t.Context(), cfg, &interactiveView{writer: &out}, false); !ready {
		t.Fatal("login not rechecked")
	}
	if calls != 4 || strings.Contains(out.String(), "fallback planner") || strings.Contains(out.String(), "fallback reviewer") {
		t.Fatal(calls, out.String())
	}
}
func TestLoginUsesSelectedExecutableAndDirectory(t *testing.T) {
	cfg := config.Defaults()
	cfg.Mode = "direct"
	cfg.Implementer = config.DefaultImplementer("codex")
	cfg.Implementer.Executable = "/custom/codex"
	cfg.WorkingDir = "/workspace/project"
	h := &Handler{}
	called := false
	h.SetConfiguredAccountLogin(func(ctx context.Context, r account.Request) error {
		called = true
		if r.Executable != "/custom/codex" || r.Directory != cfg.WorkingDir {
			t.Fatal(r)
		}
		return nil
	})
	if err := h.loginSelected(t.Context(), cfg, "codex"); err != nil || !called {
		t.Fatal(err)
	}
	if err := h.loginSelected(t.Context(), cfg, "claude"); err == nil {
		t.Fatal("unused login accepted")
	}
}
func TestCommandSuggestions(t *testing.T) {
	for _, tc := range []struct{ line, want string }{{"/conf", "/configuration"}, {"/login ", "/login claude"}, {"/set mode ", "/set mode team"}, {"/set reviewer-harness ", "/set reviewer-harness opencode"}, {"/set decision-enabled ", "/set decision-enabled true"}} {
		if !strings.Contains(strings.Join(CommandSuggestions(tc.line), "\n"), tc.want) {
			t.Fatal(tc.line, CommandSuggestions(tc.line))
		}
	}
	for _, line := range []string{"explain the project", "/config", "/configuration", "/login codex", "/set decision-enabled true"} {
		if len(CommandSuggestions(line)) != 0 {
			t.Fatal("complete command or task altered", line)
		}
	}
}

func TestSetupOffersEveryMissingAccountAndRechecksAfterLogin(t *testing.T) {
	cfg := config.Defaults()
	cfg.Mode = "team"
	cfg.Fallback.Mode = "disabled"
	cfg.Planner = config.DefaultPlanner("codex")
	cfg.Implementer = config.DefaultImplementer("opencode")
	cfg.Implementer.Model = "anthropic/model"
	cfg.Reviewer = config.DefaultPlanner("claude")
	var out bytes.Buffer
	h := &Handler{stdout: &out}
	logged := map[string]bool{}
	h.SetReadiness(func(ctx context.Context, r account.Request) account.Status {
		return account.Status{Ready: logged[r.Harness]}
	}, nil)
	h.SetConfiguredAccountLogin(func(ctx context.Context, r account.Request) error { logged[r.Harness] = true; return nil })
	if err := h.completeAccountSetup(t.Context(), &setupLines{[]string{"y", "yes", "Y"}}, cfg, &interactiveView{writer: &out}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codex", "opencode", "claude"} {
		if !logged[name] || !strings.Contains(out.String(), "Sign in to "+name) {
			t.Fatal(name, out.String())
		}
	}
	if !strings.Contains(out.String(), "Setup checks passed") {
		t.Fatal(out.String())
	}
}

func TestSetupPromptsForJevBeforeOneFinalReport(t *testing.T) {
	cfg := config.Defaults()
	cfg.Mode = "team"
	cfg.Decision.Enabled = true
	var out bytes.Buffer
	h := &Handler{stdout: &out}
	h.SetReadiness(func(context.Context, account.Request) account.Status {
		return account.Status{Ready: true, Detail: "signed in"}
	}, func(_ context.Context, _ config.Config, prompt bool) account.Status {
		if !prompt {
			t.Fatal("setup did not offer the missing Jev key")
		}
		out.WriteString("KEY PROMPT\n")
		return account.Status{Ready: true, Detail: "key accepted"}
	})
	if err := h.completeAccountSetup(t.Context(), &setupLines{}, cfg, &interactiveView{writer: &out}); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if strings.Count(text, "WORKFLOW READINESS") != 1 || strings.Index(text, "KEY PROMPT") > strings.Index(text, "WORKFLOW READINESS") || !strings.Contains(text, "Setup checks passed") {
		t.Fatal(text)
	}
}
func TestSetupOffersJevReplacementWhenExistingKeyCannotBeChecked(t *testing.T) {
	cfg := config.Defaults()
	cfg.Mode = "team"
	cfg.Decision.Enabled = true
	var out bytes.Buffer
	h := &Handler{stdout: &out}
	replaced := false
	h.SetReadiness(func(context.Context, account.Request) account.Status {
		return account.Status{Ready: true, Detail: "signed in"}
	}, func(context.Context, config.Config, bool) account.Status {
		return account.Status{Ready: replaced, Detail: "authentication check could not connect"}
	})
	h.SetJevKeyLogin(func(context.Context) error { replaced = true; return nil })
	if err := h.completeAccountSetup(t.Context(), &setupLines{[]string{"yes"}}, cfg, &interactiveView{writer: &out}); err != nil {
		t.Fatal(err)
	}
	if !replaced || !strings.Contains(out.String(), "Enter or replace its OpenRouter key now?") || !strings.Contains(out.String(), "Setup checks passed") {
		t.Fatal(out.String())
	}
}

func TestLoginJevAlwaysRequestsReplacement(t *testing.T) {
	cfg := config.Defaults()
	cfg.Mode = "team"
	cfg.Decision.Enabled = true
	filename := filepath.Join(t.TempDir(), "config.json")
	if err := saveInteractiveConfig(filename, cfg); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	h, err := NewHandler(func(config.Config, workflow.EventSink) (Runner, error) {
		t.Fatal("task unexpectedly started")
		return nil, nil
	}, &out, &out, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	h.SetReadiness(func(context.Context, account.Request) account.Status {
		return account.Status{Ready: true, Detail: "signed in"}
	}, func(context.Context, config.Config, bool) account.Status {
		return account.Status{Ready: true, Detail: "existing key accepted"}
	})
	calls := 0
	h.SetJevKeyLogin(func(context.Context) error { calls++; return nil })
	if code := h.Interactive(t.Context(), &setupLines{[]string{"/login jev", "/quit"}}, filename); code != ExitSuccess || calls != 1 {
		t.Fatal(code, calls, out.String())
	}
}
func TestSetupDoesNotTreatDeclinedOrUnsuccessfulLoginAsReady(t *testing.T) {
	cfg := config.Defaults()
	cfg.Implementer = config.DefaultImplementer("codex")
	for _, answer := range []string{"n", "y"} {
		var out bytes.Buffer
		h := &Handler{stdout: &out}
		calls := 0
		h.SetReadiness(func(context.Context, account.Request) account.Status { return account.Status{} }, nil)
		h.SetConfiguredAccountLogin(func(context.Context, account.Request) error { calls++; return nil })
		if err := h.completeAccountSetup(t.Context(), &setupLines{[]string{answer}}, cfg, &interactiveView{writer: &out}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "Setup checks passed") || (answer == "n" && calls > 0) {
			t.Fatal(answer, out.String())
		}
	}
}

func TestRemoteAuthenticationFailureRequiresNewLogin(t *testing.T) {
	cfg := config.Defaults()
	cfg.Implementer = config.DefaultImplementer("codex")
	var out bytes.Buffer
	h := &Handler{stdout: &out}
	view := &interactiveView{writer: &out}
	h.SetReadiness(func(context.Context, account.Request) account.Status { return account.Status{Ready: true} }, nil)
	h.SetConfiguredAccountLogin(func(context.Context, account.Request) error { return nil })
	failure := store.TaskOutput{Failure: &store.TaskFailure{Stage: store.WorkflowStageDelegation, Provider: &store.ProviderFailure{Kind: store.ProviderAuthentication}}}
	if err := h.rememberAuthenticationFailure(cfg, failure, view); err != nil {
		t.Fatal(err)
	}
	if ready, _ := h.readiness(t.Context(), cfg, view, false); ready {
		t.Fatal("stale local credentials accepted after HTTP401")
	}
	if !strings.Contains(out.String(), "/login codex") {
		t.Fatal(out.String())
	}
	if err := h.loginSelected(t.Context(), cfg, "codex"); err != nil {
		t.Fatal(err)
	}
	if ready, _ := h.readiness(t.Context(), cfg, view, false); !ready {
		t.Fatal("new login not rechecked")
	}
}

func TestSetupChecksSeparateOpenCodeProviderAccounts(t *testing.T) {
	cfg := config.Defaults()
	cfg.Mode = "team"
	cfg.Fallback.Mode = "disabled"
	cfg.Planner = config.DefaultPlanner("opencode")
	cfg.Planner.Model = "openai/model"
	cfg.Implementer = config.DefaultImplementer("opencode")
	cfg.Implementer.Model = "anthropic/model"
	cfg.Reviewer = cfg.Planner
	var out bytes.Buffer
	h := &Handler{stdout: &out}
	logged := map[string]bool{}
	calls := 0
	h.SetReadiness(func(ctx context.Context, r account.Request) account.Status {
		return account.Status{Ready: logged[r.Model]}
	}, nil)
	h.SetConfiguredAccountLogin(func(ctx context.Context, r account.Request) error { logged[r.Model] = true; calls++; return nil })
	if err := h.completeAccountSetup(t.Context(), &setupLines{[]string{"y", "y"}}, cfg, &interactiveView{writer: &out}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !logged["openai/model"] || !logged["anthropic/model"] {
		t.Fatal(calls, logged)
	}
}

func TestSuggestedSettingValuesAreValid(t *testing.T) {
	for _, name := range []string{"mode", "planner-harness", "reviewer-harness", "implementer-harness", "decision-enabled", "fallback-mode", "progress", "color"} {
		for _, suggestion := range CommandSuggestions("/set " + name + " ") {
			option, value, err := interactiveSetting(strings.TrimPrefix(suggestion, "/set "))
			if err != nil {
				t.Fatal(suggestion, err)
			}
			if _, err := config.Load("", t.TempDir(), nil, map[string]string{option.Name: value}); err != nil {
				t.Fatal(suggestion, err)
			}
		}
	}
}
