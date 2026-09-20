package cli

import (
	"context"
	"fmt"
	"maps"
	"strings"

	"multiharness-core/internal/config"
)

func (h *Handler) configureContainer(ctx context.Context, input LineInput, filename, settingsPath string, overrides map[string]string, cfg config.Config, view *interactiveView) (config.Config, error) {
	for {
		label := "Agent team - planner, implementer and reviewer"
		mode := "Team - separate planning, implementation and review"
		if cfg.Mode == "direct" {
			label = "Agent - one CLI handles the task"
			mode = "Direct - one agent handles the task"
		}
		message := "\n" + view.paragraph("CONFIGURATION", 2, "1;36") + "  " + view.rule() + "\n" +
			view.detailRow("Current mode", mode, "2") + "\n" +
			view.detailRow("1. Project", "Project folder - choose where tasks run", "1;36") +
			view.detailRow("2. Agents", label, "1;36") +
			view.detailRow("", "Choose each agent's CLI, model and reasoning/variant", "2") +
			view.detailRow("3. Permissions", "Agent permissions - access allowed for the selected agent", "1;36") +
			view.detailRow("4. Mode", "Execution mode - Direct (one agent) or Team (separate roles)", "1;36") + "\n" +
			view.paragraph("Menu changes save automatically. /cancel keeps the current settings.", 4, "2") +
			view.detailRow("/configuration", "Check account readiness", "36") +
			view.detailRow("/settings", "Show current values", "36") +
			view.detailRow("/options", "All available controls: timeouts, progress, validation checks, repair limits and retries", "36") +
			view.paragraph("Change advanced settings with /set OPTION VALUE, then /save.", 4, "2") +
			"\n  " + view.paint("Choose a number, or /cancel: ", "36")
		if err := view.write(message); err != nil {
			return cfg, err
		}
		choice, err := input.ReadLine(ctx, cfg.MaxTaskBytes)
		if err != nil {
			return cfg, err
		}
		switch strings.TrimSpace(choice) {
		case "1":
			updated, selected, err := h.selectWorkspace(ctx, input, cfg, view)
			if err != nil || !selected {
				return cfg, err
			}
			if err := h.rememberWorkspace(settingsPath, updated.WorkingDir); err != nil {
				return cfg, err
			}
			overrides["workdir"], overrides["session-id"] = updated.WorkingDir, ""
			return updated, view.notice("Project folder saved automatically.", false)
		case "2":
			updated, _, err := h.configureInteractive(ctx, input, filename, settingsPath, overrides, cfg, view)
			return updated, err
		case "3":
			return h.configurePermissions(ctx, input, "", filename, settingsPath, overrides, cfg, view)
		case "4":
			return h.configureMode(ctx, input, filename, settingsPath, overrides, cfg, view)
		case "/cancel":
			return cfg, nil
		default:
			if err := view.notice("Choose 1 for the project folder, 2 for agents, 3 for permissions, or 4 for Direct/Team mode.", true); err != nil {
				return cfg, err
			}
		}
	}
}

func (h *Handler) configureMode(ctx context.Context, input LineInput, filename, settingsPath string, overrides map[string]string, cfg config.Config, view *interactiveView) (config.Config, error) {
	for {
		message := "\n" + view.paragraph("EXECUTION MODE", 2, "1;36") + "  " + view.rule() + "\n" +
			view.detailRow("1. Direct", "Delegate the task to one selected CLI", "1;36") +
			view.detailRow("2. Team", "Configure a planner, implementer and reviewer separately", "1;36") + "\n" +
			view.paragraph("Switching modes starts a new conversation. Team uses restricted role permissions.", 4, "2") +
			view.paragraph("Choose 1 or 2; Enter or /cancel keeps the current mode:", 2, "36")
		if err := view.write(message); err != nil {
			return cfg, err
		}
		value, err := input.ReadLine(ctx, cfg.MaxTaskBytes)
		if err != nil {
			return cfg, err
		}
		var mode string
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "", "/cancel":
			return cfg, nil
		case "1", "direct":
			mode = "direct"
		case "2", "team":
			mode = "team"
		default:
			if err := view.notice("Choose Direct or Team using 1 or 2.", true); err != nil {
				return cfg, err
			}
			continue
		}
		candidate := maps.Clone(overrides)
		candidate["mode"] = mode
		if mode != cfg.Mode {
			candidate["session-id"] = ""
		}
		updated, err := config.Load(filename, h.baseDir, h.lookupEnv, candidate)
		if err != nil {
			return cfg, fmt.Errorf("mode not changed: %w. Use /permissions native before switching to Team if the current permissions are incompatible", err)
		}
		if err := ctx.Err(); err != nil {
			return cfg, err
		}
		if err := saveInteractiveConfig(settingsPath, updated); err != nil {
			return cfg, fmt.Errorf("cannot save mode; current settings kept: %w", err)
		}
		maps.Copy(overrides, candidate)
		message = "Mode saved: " + mode + ". "
		if mode == "team" {
			message += "Use /config > 2 to choose your planner, implementer and reviewer."
		} else {
			message += "Use /config > 2 to choose your agent."
		}
		if mode != cfg.Mode {
			message += " A new conversation will start with your next task."
		}
		return updated, view.notice(message, false)
	}
}
