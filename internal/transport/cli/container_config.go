package cli

import (
	"context"
	"strings"

	"multiharness-core/internal/config"
)

func (h *Handler) configureContainer(ctx context.Context, input LineInput, filename, settingsPath string, overrides map[string]string, cfg config.Config, view *interactiveView) (config.Config, error) {
	for {
		label := "Agent team"
		if cfg.Mode == "direct" {
			label = "Agent"
		}
		if err := interactiveWrite(h.stdout, "\n  CONFIGURATION\n  1. Project folder\n  2. "+label+"\n  3. Agent permissions\n  Choose a number, or /cancel: "); err != nil {
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
		case "/cancel":
			return cfg, nil
		default:
			if err := view.notice("Choose 1 for the project folder, 2 for the agent, or 3 for its permissions.", true); err != nil {
				return cfg, err
			}
		}
	}
}
