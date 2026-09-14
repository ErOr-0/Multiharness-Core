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
		message := "\n  CONFIGURATION\n  Current mode: " + mode + "\n\n" +
			"  1. Project folder - choose where tasks run\n" +
			"  2. " + label + "\n     Choose each agent's CLI, model and reasoning/variant\n" +
			"  3. Agent permissions - access allowed for the selected agent\n" +
			"  4. Execution mode - Direct (one agent) or Team (separate roles)\n\n" +
			"  Menu changes save automatically. /cancel keeps the current settings.\n" +
			"  /settings shows current values; /options lists all available controls.\n" +
			"  Advanced: timeouts and progress; Team also uses validation checks, repair limits and retries.\n" +
			"  Change these with /set OPTION VALUE, then /save.\n" +
			"  Choose a number, or /cancel: "
		if err := interactiveWrite(h.stdout, message); err != nil {
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
		message := "\n  EXECUTION MODE\n  1. Direct - delegate the task to one selected CLI\n" +
			"  2. Team - configure a planner, implementer and reviewer separately\n" +
			"  Switching modes starts a new conversation. Team uses restricted role permissions.\n" +
			"  Choose 1 or 2; Enter or /cancel keeps the current mode: "
		if err := interactiveWrite(h.stdout, message); err != nil {
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
