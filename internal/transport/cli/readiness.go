package cli

import (
	"context"
	"fmt"
	"strings"

	"multiharness-core/internal/adapter/account"
	decisionadapter "multiharness-core/internal/adapter/decision/openrouter"
	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/screen"
	"multiharness-core/internal/transport/cli/term"
)

// SetReadiness connects bounded account checks, keeping provider processes out
// of the terminal transport. The bool requests hidden decision key entry during
// setup.
func (h *Handler) SetReadiness(agent func(context.Context, account.Request) account.Status, decision func(context.Context, config.Config, bool) account.Status) {
	h.checkAccount = agent
	h.checkDecision = decision
}

// SetDecisionKeyLogin supplies hidden key entry for the configured decision
// provider (/login jev or /login laya).
func (h *Handler) SetDecisionKeyLogin(login func(context.Context, config.Decision) error) {
	h.loginDecisionKey = login
}
func (h *Handler) SetConfiguredAccountLogin(login func(context.Context, account.Request) error) {
	h.configuredLogin = login
}

func (h *Handler) readiness(ctx context.Context, cfg config.Config, view *screen.View, prompt bool) (bool, error) {
	return h.readinessWithAccounts(ctx, cfg, view, prompt, nil)
}

// Tasks still recheck every required account, but a healthy run should go
// straight to progress instead of printing the same setup panel each time.
func (h *Handler) readinessForTask(ctx context.Context, cfg config.Config, view *screen.View) (bool, error) {
	var report strings.Builder
	preview := *view
	preview.Writer = &report
	preview.Width = view.ContentWidth()
	ready, err := h.readiness(ctx, cfg, &preview, true)
	if err != nil || ready {
		return ready, err
	}
	return false, term.Write(view.Writer, report.String())
}

func (h *Handler) readinessWithAccounts(ctx context.Context, cfg config.Config, view *screen.View, prompt bool, checked map[account.Request]account.Status) (bool, error) {
	if h.checkAccount == nil {
		return true, nil
	}
	// Ask for a missing decision key before drawing the report, so a
	// hidden-input prompt cannot interrupt it halfway through.
	decisionName := cfg.Decision.ProviderName()
	decisionStatus := account.Status{Detail: decisionName + " check unavailable"}
	if cfg.Mode == "team" && cfg.Decision.Enabled && h.checkDecision != nil {
		decisionStatus = h.checkDecision(ctx, cfg, prompt)
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := view.ReadinessHeader(cfg.Mode); err != nil {
		return false, err
	}
	ready := true
	if checked == nil {
		checked = map[account.Request]account.Status{}
	}
	for _, item := range cfg.RoleAgents() {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		r := account.Request{Harness: item.Agent.Harness, Executable: item.Agent.Executable, Model: item.Agent.Model, Directory: cfg.WorkingDir, InstallMode: cfg.InstallMode}
		status, ok := checked[r]
		if !ok {
			status = h.checkAccount(ctx, r)
			checked[r] = status
		}
		if h.rejectedAccounts[r] {
			status = account.Status{Detail: "Provider rejected authentication on the last task; use /login " + r.Harness + " before retrying"}
		}
		if !status.Ready {
			ready = false
		}
		if err := view.ReadinessAgent(item.Role, screen.HarnessName(r.Harness), r.Model, status); err != nil {
			return false, err
		}
	}
	if err := view.Print("\n" + view.Paragraph("OPTIONAL SERVICES", 2, "1")); err != nil {
		return false, err
	}
	if cfg.Mode == "team" && cfg.Decision.Enabled {
		if !decisionStatus.Ready {
			ready = false
		}
		decision := cfg.Decision.Effective()
		other := decisionadapter.OtherProvider(decision.Provider)
		value := fmt.Sprintf("%s · %s · or %s via /set decision-provider %s", decision.Model, decision.Endpoint, decisionadapter.ProviderName(other), other)
		if err := view.Print(view.DetailRow(decisionName+" routing", value, "1;36") + view.ReadinessStatus(decisionStatus)); err != nil {
			return false, err
		}
	} else {
		if err := view.Print(view.DetailRow("Decision routing", "Off · NOT REQUIRED for this workflow · choose jev (hosted) or laya (self-hosted) with /set decision-provider", "2")); err != nil {
			return false, err
		}
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return ready, view.ReadinessSummary(ready)
}

func (h *Handler) loginSelected(ctx context.Context, cfg config.Config, provider string) error {
	if h.configuredLogin == nil {
		return fmt.Errorf("account login is unavailable")
	}
	var selected *account.Request
	for _, item := range cfg.RoleAgents() {
		if item.Agent.Harness != provider {
			continue
		}
		r := account.Request{Harness: provider, Executable: item.Agent.Executable, Model: item.Agent.Model, Directory: cfg.WorkingDir, InstallMode: cfg.InstallMode}
		if selected != nil && selected.Executable != r.Executable {
			return fmt.Errorf("multiple %s executables selected; sign in with each executable or choose one in /config", provider)
		}
		if selected == nil {
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
		return fmt.Errorf("account login is unavailable")
	}
	if err := h.configuredLogin(ctx, request); err != nil {
		return err
	}
	for previous := range h.rejectedAccounts {
		if previous.Harness == request.Harness && previous.Executable == request.Executable {
			delete(h.rejectedAccounts, previous)
		}
	}
	return nil
}

// completeAccountSetup offers each missing selected account immediately after
// configuration. Declining keeps the settings, but never permits an unready task.
func (h *Handler) completeAccountSetup(ctx context.Context, input LineInput, cfg config.Config, view *screen.View) error {
	if h.checkAccount == nil {
		return nil
	}
	checked := map[account.Request]account.Status{}
	prompted := map[string]bool{}
	for _, item := range cfg.RoleAgents() {
		if err := ctx.Err(); err != nil {
			return err
		}
		r := account.Request{Harness: item.Agent.Harness, Executable: item.Agent.Executable, Model: item.Agent.Model, Directory: cfg.WorkingDir, InstallMode: cfg.InstallMode}
		status, ok := checked[r]
		if !ok {
			status = h.checkAccount(ctx, r)
			checked[r] = status
		}
		promptKey := r.Harness + "\x00" + r.Executable
		if (status.Ready && !h.rejectedAccounts[r]) || prompted[promptKey] {
			continue
		}
		prompted[promptKey] = true
		if err := term.Write(h.stdout, "\n  Sign in to "+terminalText(r.Harness)+" now? [y/N] (you can also use /login "+terminalText(r.Harness)+"): "); err != nil {
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
				if err := view.Notice("Sign-in did not finish. Use /login "+r.Harness+" to retry.", true); err != nil {
					return err
				}
			}
			delete(checked, r)
		}
	}
	// Recheck accounts that were missing, including declined sign-ins. Accounts
	// already ready can be reused without launching their CLIs a second time.
	for r, status := range checked {
		if !status.Ready {
			delete(checked, r)
		}
	}
	if cfg.Mode == "team" && cfg.Decision.Enabled && h.checkDecision != nil && h.loginDecisionKey != nil {
		// A key can only fix a missing or rejected credential. An unreachable
		// Laya container is reported, not prompted for.
		status := h.checkDecision(ctx, cfg, false)
		if !status.Ready && (cfg.Decision.RequiresKey() || strings.Contains(status.Detail, "key")) {
			decision := cfg.Decision.Effective()
			keyOwner := "server"
			if cfg.Decision.RequiresKey() {
				keyOwner = "OpenRouter"
			}
			if err := term.Write(h.stdout, fmt.Sprintf("\n  %s is not ready. Enter or replace its %s key now? [y/N] (or use /login %s later): ", decision.ProviderName(), keyOwner, decision.Provider)); err != nil {
				return err
			}
			answer, err := input.ReadLine(ctx, 64)
			if err != nil {
				return err
			}
			if strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes") {
				if err := h.loginDecisionKey(ctx, cfg.Decision); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if err := view.Notice(fmt.Sprintf("%s key entry did not finish. Use /login %s to retry.", decision.ProviderName(), decision.Provider), true); err != nil {
						return err
					}
				}
			}
		}
	}
	_, err := h.readinessWithAccounts(ctx, cfg, view, h.loginDecisionKey == nil, checked)
	return err
}

// Remember a remote authentication rejection even if a CLI still has stale
// local credentials. Never automatically replay the user's task after login.
func (h *Handler) rememberAuthenticationFailure(cfg config.Config, output contract.TaskOutput, view *screen.View) error {
	if output.Failure == nil || output.Failure.Provider == nil || output.Failure.Provider.Kind != contract.ProviderAuthentication {
		return nil
	}
	role := "agent"
	if cfg.Mode == "team" {
		switch output.Failure.Stage {
		case contract.WorkflowStagePlanning, contract.WorkflowStageAnswering:
			role = "planner"
		case contract.WorkflowStageImplementation, contract.WorkflowStageRepair:
			role = "implementer"
		case contract.WorkflowStageReview:
			role = "reviewer"
		default:
			return nil
		}
	}
	for _, item := range cfg.RoleAgents() {
		if item.Role == role {
			if h.rejectedAccounts == nil {
				h.rejectedAccounts = map[account.Request]bool{}
			}
			r := account.Request{Harness: item.Agent.Harness, Executable: item.Agent.Executable, Model: item.Agent.Model, Directory: cfg.WorkingDir, InstallMode: cfg.InstallMode}
			h.rejectedAccounts[r] = true
			return view.Notice("Authentication failed for "+r.Harness+". Use /login "+r.Harness+", then /configuration. Your task will not restart automatically.", true)
		}
	}
	return nil
}
