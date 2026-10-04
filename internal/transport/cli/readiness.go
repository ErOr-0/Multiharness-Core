package cli

import (
	"context"
	"fmt"
	"strings"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/screen"
	"multiharness-core/internal/transport/cli/term"
)

// optionalFallbacks lists alternate agents a team run may switch to.
func optionalFallbacks(cfg config.Config) []config.RoleAgent {
	if cfg.Mode != "team" {
		return nil
	}
	var result []config.RoleAgent
	if cfg.Fallback.Mode != "disabled" {
		if cfg.Planner.Harness != "claude" && cfg.Planner.Harness != "muse" {
			result = append(result, config.RoleAgent{Role: "fallback planner", Agent: cfg.Fallback.Planner})
		}
		if cfg.Implementer.Harness == "opencode" {
			p := config.DefaultPlanner("codex")
			p.Executable = cfg.Fallback.CodexImplementer.Executable
			p.Model = cfg.Fallback.CodexImplementer.Model
			result = append(result, config.RoleAgent{Role: "fallback implementer", Agent: p})
		}
		if cfg.Reviewer.Harness == "codex" {
			p := config.DefaultPlanner("opencode")
			p.Executable = cfg.Fallback.OpenCodeReviewer.Executable
			p.Model = cfg.Fallback.OpenCodeReviewer.Model
			result = append(result, config.RoleAgent{Role: "fallback reviewer", Agent: p})
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
func (h *Handler) SetJevKeyLogin(login func(context.Context) error) { h.loginJev = login }
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
	// Ask for a missing Jev key before drawing the report, so a hidden-input
	// prompt cannot interrupt it halfway through.
	jevStatus := account.Status{Detail: "Jev account check unavailable"}
	if cfg.Mode == "team" && cfg.Decision.Enabled && h.checkJev != nil {
		jevStatus = h.checkJev(ctx, cfg, prompt)
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
		if item.Agent.Harness == "opencode" && item.Agent.Model == "" {
			option := strings.ReplaceAll(item.Role, " ", "-") + "-model"
			if item.Role == "agent" {
				option = "implementer-model"
			}
			if item.Role == "fallback reviewer" {
				option = "fallback-opencode-reviewer-model"
			}
			status.Detail = "Choose the provider/model with /set " + option + " provider/model, then /login opencode if required"
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
		if !jevStatus.Ready {
			ready = false
		}
		if err := view.Print(view.DetailRow("Jev routing", cfg.Decision.Model, "1;36") + view.ReadinessStatus(jevStatus)); err != nil {
			return false, err
		}
	} else {
		if err := view.Print(view.DetailRow("Jev routing", "Off · NOT REQUIRED for this workflow", "2")); err != nil {
			return false, err
		}
	}
	fallback := "Off · only your selected agents will run"
	if cfg.Mode == "team" && cfg.Fallback.Mode != "disabled" {
		fallback = "Opted in · alternate account checked only after you accept a switch"
	}
	if err := view.Print(view.DetailRow("Fallbacks", fallback, "2")); err != nil {
		return false, err
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
	for _, item := range append(cfg.RoleAgents(), optionalFallbacks(cfg)...) {
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
		return fmt.Errorf("account login is unavailable")
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
		provider, _, _ := strings.Cut(r.Model, "/")
		promptKey := r.Harness + "\x00" + r.Executable
		label := r.Harness
		if r.Harness == "opencode" {
			promptKey += "\x00" + provider
			label += " (" + provider + ")"
		}
		if (status.Ready && !h.rejectedAccounts[r]) || prompted[promptKey] {
			continue
		}
		prompted[promptKey] = true
		if r.Harness == "opencode" && r.Model == "" {
			continue
		}
		if err := term.Write(h.stdout, "\n  Sign in to "+terminalText(label)+" now? [y/N] (you can also use /login "+terminalText(r.Harness)+"): "); err != nil {
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
	if cfg.Mode == "team" && cfg.Decision.Enabled && h.checkJev != nil && h.loginJev != nil {
		if status := h.checkJev(ctx, cfg, false); !status.Ready {
			if err := term.Write(h.stdout, "\n  Jev is not ready. Enter or replace its OpenRouter key now? [y/N] (or use /login jev later): "); err != nil {
				return err
			}
			answer, err := input.ReadLine(ctx, 64)
			if err != nil {
				return err
			}
			if strings.EqualFold(strings.TrimSpace(answer), "y") || strings.EqualFold(strings.TrimSpace(answer), "yes") {
				if err := h.loginJev(ctx); err != nil {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					if err := view.Notice("Jev key entry did not finish. Use /login jev to retry.", true); err != nil {
						return err
					}
				}
			}
		}
	}
	_, err := h.readinessWithAccounts(ctx, cfg, view, h.loginJev == nil, checked)
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
		for _, change := range output.AgentSwitches {
			if change.Stage == output.Failure.Stage || (role == "implementer" && change.Stage == contract.WorkflowStageImplementation) {
				role = "fallback " + role
				break
			}
		}
	}
	for _, item := range append(cfg.RoleAgents(), optionalFallbacks(cfg)...) {
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
