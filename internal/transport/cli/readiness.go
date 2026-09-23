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
	return h.readinessWithAccounts(ctx, cfg, view, prompt, nil)
}

// Tasks still recheck every required account, but a healthy run should go
// straight to progress instead of printing the same setup panel each time.
func (h *Handler) readinessForTask(ctx context.Context, cfg config.Config, view *interactiveView) (bool, error) {
	var report strings.Builder
	preview := *view
	preview.writer = &report
	preview.width = view.contentWidth()
	ready, err := h.readiness(ctx, cfg, &preview, true)
	if err != nil || ready {
		return ready, err
	}
	return false, interactiveWrite(view.writer, report.String())
}

func (h *Handler) readinessWithAccounts(ctx context.Context, cfg config.Config, view *interactiveView, prompt bool, checked map[account.Request]account.Status) (bool, error) {
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
	if err := view.readinessHeader(cfg.Mode); err != nil {
		return false, err
	}
	ready := true
	if checked == nil {
		checked = map[account.Request]account.Status{}
	}
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
		if !status.Ready {
			ready = false
		}
		if err := view.readinessAgent(item.role, harnessName(r.Harness), r.Model, status); err != nil {
			return false, err
		}
	}
	if err := view.write("\n" + view.paragraph("OPTIONAL SERVICES", 2, "1")); err != nil {
		return false, err
	}
	if cfg.Mode == "team" && cfg.Decision.Enabled {
		if !jevStatus.Ready {
			ready = false
		}
		if err := view.write(view.detailRow("Jev routing", cfg.Decision.Model, "1;36") + view.readinessStatus(jevStatus)); err != nil {
			return false, err
		}
	} else {
		if err := view.write(view.detailRow("Jev routing", "Off · NOT REQUIRED for this workflow", "2")); err != nil {
			return false, err
		}
	}
	fallback := "Off · only your selected agents will run"
	if cfg.Mode == "team" && cfg.Fallback.Mode != "disabled" {
		fallback = "Opted in · alternate account checked only after you accept a switch"
	}
	if err := view.write(view.detailRow("Fallbacks", fallback, "2")); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return ready, view.readinessSummary(ready)
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
	checked := map[account.Request]account.Status{}
	prompted := map[string]bool{}
	for _, item := range requirements(cfg) {
		if err := ctx.Err(); err != nil {
			return err
		}
		r := account.Request{Harness: item.agent.Harness, Executable: item.agent.Executable, Model: item.agent.Model, Directory: cfg.WorkingDir, InstallMode: cfg.InstallMode}
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
	_, err := h.readinessWithAccounts(ctx, cfg, view, true, checked)
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
		case store.WorkflowStagePlanning, store.WorkflowStageAnswering:
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
