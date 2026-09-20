package cli

import (
	"context"
	"fmt"
	"strings"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

type requirement struct {
	role  string
	agent config.Planner
}

func requirements(cfg config.Config) []requirement {
	if cfg.Mode == "direct" {
		return []requirement{{"agent", config.Planner(cfg.Implementer)}}
	}
	return []requirement{{"planner", cfg.Planner}, {"implementer", config.Planner(cfg.Implementer)}, {"reviewer", cfg.Reviewer}}
}

func optionalFallbacks(cfg config.Config) []requirement {
	if cfg.Mode != "team" {
		return nil
	}
	var result []requirement
	if cfg.Fallback.Mode != "disabled" {
		if cfg.Planner.Harness != "claude" {
			result = append(result, requirement{"fallback planner", cfg.Fallback.Planner})
		}
		if cfg.Implementer.Harness == "opencode" {
			p := config.DefaultPlanner("codex")
			p.Executable = cfg.Fallback.CodexImplementer.Executable
			p.Model = cfg.Fallback.CodexImplementer.Model
			result = append(result, requirement{"fallback implementer", p})
		}
		if cfg.Reviewer.Harness == "codex" {
			p := config.DefaultPlanner("opencode")
			p.Executable = cfg.Fallback.OpenCodeReviewer.Executable
			p.Model = cfg.Fallback.OpenCodeReviewer.Model
			result = append(result, requirement{"fallback reviewer", p})
		}
	}
	return result
}

// SetReadiness connects bounded account checks, keeping provider processes out
// of the terminal transport. The bool requests hidden Jev key entry during setup.
func (h *Handler) SetReadiness(agent func(context.Context, account.Request) account.Status, jev func(context.Context, config.Config, bool) account.Status) {
	h.checkAccount = agent
	h.checkJev = jev
}
func (h *Handler) SetConfiguredAccountLogin(login func(context.Context, account.Request) error) {
	h.configuredLogin = login
}

func (h *Handler) readiness(ctx context.Context, cfg config.Config, view *interactiveView, prompt bool) (bool, error) {
	if h.checkAccount == nil {
		return true, nil
	}
	if err := interactiveWrite(h.stdout, "\n  WORKFLOW READINESS · "+terminalText(cfg.Mode)+"\n"); err != nil {
		return false, err
	}
	ready := true
	checked := map[account.Request]account.Status{}
	for _, item := range requirements(cfg) {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		r := account.Request{Harness: item.agent.Harness, Executable: item.agent.Executable, Model: item.agent.Model, Directory: cfg.WorkingDir, InstallMode: cfg.InstallMode}
		status, ok := checked[r]
		if !ok {
			status = h.checkAccount(ctx, r)
			checked[r] = status
		}
		if h.rejectedAccounts[r] {
			status = account.Status{Detail: "Provider rejected authentication on the last task; use /login " + r.Harness + " before retrying"}
		}
		if item.agent.Harness == "opencode" && item.agent.Model == "" {
			option := strings.ReplaceAll(item.role, " ", "-") + "-model"
			if item.role == "agent" {
				option = "implementer-model"
			}
			if item.role == "fallback reviewer" {
				option = "fallback-opencode-reviewer-model"
			}
			status.Detail = "Choose the provider/model with /set " + option + " provider/model, then /login opencode if required"
		}
		label := "READY"
		if !status.Ready {
			label = "NEEDS SETUP"
			ready = false
		}
		if err := interactiveWrite(h.stdout, fmt.Sprintf("  [%s] %s · %s · %s\n    %s\n", label, terminalText(item.role), terminalText(r.Harness), terminalText(r.Model), terminalText(status.Detail))); err != nil {
			return false, err
		}
	}
	if cfg.Mode == "team" && cfg.Decision.Enabled {
		status := account.Status{Detail: "Jev account check unavailable"}
		if h.checkJev != nil {
			status = h.checkJev(ctx, cfg, prompt)
		}
		label := "READY"
		if !status.Ready {
			label = "NEEDS SETUP"
			ready = false
		}
		if err := interactiveWrite(h.stdout, fmt.Sprintf("  [%s] Jev · %s\n    %s\n", label, terminalText(cfg.Decision.Model), terminalText(status.Detail))); err != nil {
			return false, err
		}
	} else {
		if err := interactiveWrite(h.stdout, "  [NOT REQUIRED] Jev · disabled for this workflow\n"); err != nil {
			return false, err
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	message := "Setup checks passed. Native CLI checks do not guarantee remote model access or available credits."
	if !ready {
		message = "Setup incomplete. Tasks are blocked. Follow the steps above, then use /configuration to check again. /config changes your selection."
	}
	if cfg.Mode == "team" && cfg.Fallback.Mode != "disabled" {
		message += " Fallbacks are optional: an alternate account is checked only if you accept a switch after a provider usage-limit failure."
	}
	return ready, view.notice(message, !ready)
}

func (h *Handler) loginSelected(ctx context.Context, cfg config.Config, provider string) error {
	if h.configuredLogin == nil {
		if h.accountLogin == nil {
			return fmt.Errorf("account login is unavailable")
		}
		return h.accountLogin(ctx, provider)
	}
	var selected *account.Request
	for _, item := range append(requirements(cfg), optionalFallbacks(cfg)...) {
		if item.agent.Harness != provider {
			continue
		}
		r := account.Request{Harness: provider, Executable: item.agent.Executable, Model: item.agent.Model, Directory: cfg.WorkingDir, InstallMode: cfg.InstallMode}
		if selected != nil && selected.Executable != r.Executable {
			return fmt.Errorf("multiple %s executables selected; sign in with each executable or choose one in /config", provider)
		}
		if selected == nil {
			selected = &r
		}
		if provider == "opencode" && h.checkAccount != nil && !h.checkAccount(ctx, r).Ready {
			selected = &r
		}
	}
	if selected == nil {
		return fmt.Errorf("%s is not selected in this workflow; choose it with /config first", strings.TrimSpace(provider))
	}
	return h.loginRequest(ctx, *selected)
}

func (h *Handler) loginRequest(ctx context.Context, request account.Request) error {
	if h.configuredLogin == nil {
		if h.accountLogin == nil {
			return fmt.Errorf("account login is unavailable")
		}
		return h.accountLogin(ctx, request.Harness)
	}
	if err := h.configuredLogin(ctx, request); err != nil {
		return err
	}
	provider, _, _ := strings.Cut(request.Model, "/")
	for previous := range h.rejectedAccounts {
		previousProvider, _, _ := strings.Cut(previous.Model, "/")
		if previous.Harness == request.Harness && previous.Executable == request.Executable && (request.Harness != "opencode" || previousProvider == provider) {
			delete(h.rejectedAccounts, previous)
		}
	}
	return nil
}

// completeAccountSetup offers each missing selected account immediately after
// configuration. Declining keeps the settings, but never permits an unready task.
func (h *Handler) completeAccountSetup(ctx context.Context, input LineInput, cfg config.Config, view *interactiveView) error {
	if h.checkAccount == nil {
		return nil
	}
	ready, err := h.readiness(ctx, cfg, view, false)
	if err != nil || ready {
		return err
	}
	prompted := map[string]bool{}
	for _, item := range requirements(cfg) {
		r := account.Request{Harness: item.agent.Harness, Executable: item.agent.Executable, Model: item.agent.Model, Directory: cfg.WorkingDir, InstallMode: cfg.InstallMode}
		provider, _, _ := strings.Cut(r.Model, "/")
		promptKey := r.Harness + "\x00" + r.Executable
		label := r.Harness
		if r.Harness == "opencode" {
			promptKey += "\x00" + provider
			label += " (" + provider + ")"
		}
		if (h.checkAccount(ctx, r).Ready && !h.rejectedAccounts[r]) || prompted[promptKey] {
			continue
		}
		prompted[promptKey] = true
		if r.Harness == "opencode" && r.Model == "" {
			continue
		}
		if err := interactiveWrite(h.stdout, "\n  Sign in to "+terminalText(label)+" now? [y/N] (you can also use /login "+terminalText(r.Harness)+"): "); err != nil {
			return err
		}
		answer, err := input.ReadLine(ctx, 64)
		if err != nil {
			return err
		}
		if strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes") {
			if err := h.loginRequest(ctx, r); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if err := view.notice("Sign-in did not finish. Use /login "+r.Harness+" to retry.", true); err != nil {
					return err
				}
			}
		}
	}
	_, err = h.readiness(ctx, cfg, view, true)
	return err
}

// Remember a remote authentication rejection even if a CLI still has stale
// local credentials. Never automatically replay the user's task after login.
func (h *Handler) rememberAuthenticationFailure(cfg config.Config, output store.TaskOutput, view *interactiveView) error {
	if output.Failure == nil || output.Failure.Provider == nil || output.Failure.Provider.Kind != store.ProviderAuthentication {
		return nil
	}
	role := "agent"
	if cfg.Mode == "team" {
		switch output.Failure.Stage {
		case store.WorkflowStagePlanning:
			role = "planner"
		case store.WorkflowStageImplementation, store.WorkflowStageRepair:
			role = "implementer"
		case store.WorkflowStageReview:
			role = "reviewer"
		default:
			return nil
		}
		for _, change := range output.AgentSwitches {
			if change.Stage == output.Failure.Stage || (role == "implementer" && change.Stage == store.WorkflowStageImplementation) {
				role = "fallback " + role
				break
			}
		}
	}
	for _, item := range append(requirements(cfg), optionalFallbacks(cfg)...) {
		if item.role == role {
			if h.rejectedAccounts == nil {
				h.rejectedAccounts = map[account.Request]bool{}
			}
			r := account.Request{Harness: item.agent.Harness, Executable: item.agent.Executable, Model: item.agent.Model, Directory: cfg.WorkingDir, InstallMode: cfg.InstallMode}
			h.rejectedAccounts[r] = true
			return view.notice("Authentication failed for "+r.Harness+". Use /login "+r.Harness+", then /configuration. Your task will not restart automatically.", true)
		}
	}
	return nil
}
