package cli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"multiharness-core/internal/config"
)

func permissionDescription(cfg config.Config) string {
	if cfg.Implementer.PermissionPolicy == "auto_approve" {
		return "Auto-approve requests (--auto); explicit OpenCode deny rules still apply"
	}
	return "Native rules; permission requests are rejected in non-interactive runs"
}

// configurePermissions changes existing application settings. Native execution
// flags remain the provider adapter's responsibility; no provider files are edited.
func (h *Handler) configurePermissions(ctx context.Context, input LineInput, value, filename, settingsPath string, overrides map[string]string, cfg config.Config, view *interactiveView) (config.Config, error) {
	if cfg.Implementer.Harness != "opencode" {
		return cfg, errors.New("/permissions currently configures OpenCode. Select OpenCode as your agent first")
	}
	menu := value == ""
	for {
		if value == "" {
			message := "\n  OPENCODE PERMISSIONS\n  Current: " + permissionDescription(cfg) + "\n\n  1. Native rules: reject requests that need approval\n  2. Auto-approve permission requests (--auto)\n     Applies to all permission requests, including paths outside the project.\n     Explicit deny rules in OpenCode still apply.\n  Choice saves automatically. Enter or /cancel keeps the current setting.\n  Choose 1 or 2: "
			if err := interactiveWrite(h.stdout, message); err != nil {
				return cfg, err
			}
			line, err := input.ReadLine(ctx, cfg.MaxTaskBytes)
			if err != nil {
				return cfg, err
			}
			value = strings.TrimSpace(line)
			if value == "" || strings.EqualFold(value, "/cancel") {
				return cfg, nil
			}
		}
		var policy string
		switch strings.ToLower(value) {
		case "1", "native", "reject_on_prompt":
			policy = "reject_on_prompt"
		case "2", "auto", "auto_approve":
			policy = "auto_approve"
		default:
			if menu {
				if err := view.notice("Choose 1 or 2, or /cancel to keep your permissions.", true); err != nil {
					return cfg, err
				}
				value = ""
				continue
			}
			return cfg, errors.New("use /permissions, /permissions native or /permissions auto")
		}
		candidate := maps.Clone(overrides)
		candidate["implementer-permission-policy"] = policy
		updated, err := config.Load(filename, h.baseDir, h.lookupEnv, candidate)
		if err != nil {
			return cfg, err
		}
		if err := ctx.Err(); err != nil {
			return cfg, err
		}
		if err := saveInteractiveConfig(settingsPath, updated); err != nil {
			return cfg, fmt.Errorf("cannot save permissions; current settings kept: %w", err)
		}
		maps.Copy(overrides, candidate)
		message := "OpenCode permissions saved: " + permissionDescription(updated) + ". The next task uses this setting."
		if cfg.Mode == "direct" {
			message += " Your current conversation is kept; retry the blocked task when ready."
		}
		return updated, view.notice(message, false)
	}
}
