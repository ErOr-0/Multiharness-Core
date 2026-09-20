package cli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"

	"multiharness-core/internal/config"
)

func permissionDescription(cfg config.Config) string {
	choice := cfg.CurrentPermission()
	return choice.Label + ": " + choice.Detail
}

// configurePermissions changes existing application settings. Native execution
// flags remain the provider adapter's responsibility; no provider files are edited.
func (h *Handler) configurePermissions(ctx context.Context, input LineInput, value, filename, settingsPath string, overrides map[string]string, cfg config.Config, view *interactiveView) (config.Config, error) {
	choices := cfg.PermissionChoices()
	if len(choices) == 0 {
		return cfg, errors.New("the selected agent does not expose supported permission settings")
	}
	label := harnessName(cfg.Implementer.Harness)
	names := make([]string, len(choices))
	for i, choice := range choices {
		names[i] = choice.Name
	}
	usage := "Use /permissions or /permissions " + strings.Join(names, "|")
	prompt := fmt.Sprintf("Choose 1 to %d: ", len(choices))
	menu := value == ""
	for {
		if value == "" {
			message := "\n" + view.paragraph(strings.ToUpper(label)+" PERMISSIONS", 2, "1;36") + "  " + view.rule() + "\n" + view.paragraph("Current: "+permissionDescription(cfg), 4, "2") + "\n"
			for i, choice := range choices {
				message += view.paragraph(fmt.Sprintf("%d. %s (%s)", i+1, choice.Label, choice.Name), 4, "1;36") + view.paragraph(choice.Detail, 6, "0")
			}
			if cfg.Mode == "team" {
				message += view.paragraph("Team mode keeps role-specific permissions; direct mode exposes all native modes.", 4, "2")
			}
			message += view.paragraph("Choice saves automatically. Enter or /cancel keeps the current setting.", 4, "2") + "  " + view.paint(prompt, "36")
			if err := view.write(message); err != nil {
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
		selected := -1
		if strings.EqualFold(value, "native") {
			selected = 0
		}
		for i, choice := range choices {
			if strings.EqualFold(value, choice.Name) || value == choice.Value || value == strconv.Itoa(i+1) {
				selected = i
				break
			}
		}
		if selected < 0 {
			if menu {
				if err := view.notice(usage+", or /cancel to keep your permissions.", true); err != nil {
					return cfg, err
				}
				value = ""
				continue
			}
			return cfg, errors.New(usage)
		}
		candidate := maps.Clone(overrides)
		choice := choices[selected]
		candidate[choice.Option] = choice.Value
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
		message := label + " permissions saved: " + permissionDescription(updated) + " The next task uses this setting."
		if cfg.Mode == "direct" {
			message += " Your current conversation is kept; retry the blocked task when ready."
		}
		return updated, view.notice(message, false)
	}
}
