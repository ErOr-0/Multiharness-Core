package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
	"multiharness-core/internal/transport/cli/screen"
)

// menuModels bounds the printed list; every model stays selectable by number
// or name, and typing searches the full list.
const menuModels = 20

// SetModelCatalog connects metadata-only model discovery at the composition
// root, keeping provider processes out of the terminal transport.
func (h *Handler) SetModelCatalog(list func(context.Context, account.Request) ([]account.Model, error)) {
	h.listModels = list
}

// modelChoices is what one CLI reported. Without a catalog connection the
// wizard keeps accepting literal identifiers; an empty or failed catalog
// confirms nothing, so only the current value can be kept.
type modelChoices struct {
	harness string
	models  []account.Model
	checked bool
	failed  bool
}

func (h *Handler) modelChoices(ctx context.Context, cfg config.Config, agent config.Planner, view *screen.View, cache map[account.Request]modelChoices) (modelChoices, error) {
	choices := modelChoices{harness: agent.Harness}
	if h.listModels == nil {
		return choices, nil
	}
	r := account.Request{Harness: agent.Harness, Executable: agent.Executable, Directory: cfg.WorkingDir, InstallMode: cfg.InstallMode}
	if cached, ok := cache[r]; ok {
		return cached, nil
	}
	if agent.Harness != "claude" {
		if err := view.Print(view.Paragraph("Loading "+screen.HarnessName(agent.Harness)+" models...", 4, "2")); err != nil {
			return choices, err
		}
	}
	models, err := h.listModels(ctx, r)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return choices, ctxErr
	}
	choices.checked, choices.failed, choices.models = true, err != nil, models
	if err != nil {
		choices.models = nil
	}
	cache[r] = choices
	return choices, nil
}

func (c modelChoices) listed(model string) bool {
	for _, m := range c.models {
		if m.ID == model {
			return true
		}
	}
	return false
}

func (c modelChoices) menu(current string, width int) string {
	if !c.checked {
		return ""
	}
	if len(c.models) == 0 {
		reason := screen.HarnessName(c.harness) + " did not report any models."
		if c.failed {
			reason = screen.HarnessName(c.harness) + " models could not be loaded."
		}
		switch c.harness {
		case "muse":
			reason += " Muse lists models after sign-in: use /login muse, then /config."
		case "opencode":
			reason += " OpenCode lists models for signed-in providers: use /login opencode, then /config."
		default:
			reason += " Check the CLI with /configuration, then reopen /config."
		}
		return "\n  " + reason + "\n  Enter keeps the current model; other names cannot be confirmed, so they are not accepted."
	}
	var text strings.Builder
	limit := max(24, width-12)
	for index, m := range c.models[:min(len(c.models), menuModels)] {
		line := fmt.Sprintf("%d. %s", index+1, terminalText(m.ID))
		var notes []string
		if m.ID == current {
			notes = append(notes, "current")
		}
		if m.Default {
			notes = append(notes, "CLI default")
		}
		if len(notes) > 0 {
			line += " (" + strings.Join(notes, ", ") + ")"
		}
		if description := strings.Join(strings.Fields(terminalText(m.Description)), " "); description != "" && len([]rune(line)) < limit-8 {
			room := limit - len([]rune(line)) - 3
			if runes := []rune(description); len(runes) > room {
				description = string(runes[:room-1]) + "…"
			}
			line += " - " + description
		}
		text.WriteString("\n    " + line)
	}
	if hidden := len(c.models) - menuModels; hidden > 0 {
		fmt.Fprintf(&text, "\n    ... %d more; type part of a name to search them", hidden)
	}
	text.WriteString("\n  Choose a number or type a model name; matching names appear as you type.")
	if current != "" && !c.listed(current) {
		text.WriteString("\n  The current model is not available here, so choose one from this list.")
	} else {
		text.WriteString(" Enter keeps the current model.")
	}
	return text.String()
}

// checkModelSetting applies the wizard's catalog rule to /set ROLE-model, by
// name only since the list is not shown there. cfg already holds the change.
func (h *Handler) checkModelSetting(ctx context.Context, cfg config.Config, option, value string, view *screen.View) (string, error) {
	role, ok := strings.CutSuffix(option, "-model")
	if !ok || value == "" || h.listModels == nil {
		return value, nil
	}
	var agent config.Planner
	switch role {
	case "planner":
		agent = cfg.Planner
	case "reviewer":
		agent = cfg.Reviewer
	case "implementer":
		agent = config.Planner(cfg.Implementer)
	default:
		return value, nil
	}
	choices, err := h.modelChoices(ctx, cfg, agent, view, map[account.Request]modelChoices{})
	if err != nil {
		return "", err
	}
	if _, err := strconv.Atoi(value); err == nil && !choices.listed(value) {
		return "", fmt.Errorf("use a model name with /set; /config shows the numbered list")
	}
	return choices.selection(value)
}

// selection resolves an answer to a listed identifier. An exact identifier wins
// over a number so a numeric model name stays reachable.
func (c modelChoices) selection(value string) (string, error) {
	if !c.checked {
		return value, nil
	}
	name := screen.HarnessName(c.harness)
	if len(c.models) == 0 {
		return "", fmt.Errorf("%s did not confirm %q, so it was not accepted; press Enter to keep the current model", name, terminalText(value))
	}
	for _, m := range c.models {
		if m.ID == value {
			return m.ID, nil
		}
	}
	for _, m := range c.models {
		if strings.EqualFold(m.ID, value) {
			return m.ID, nil
		}
	}
	if n, err := strconv.Atoi(value); err == nil {
		if n >= 1 && n <= len(c.models) {
			return c.models[n-1].ID, nil
		}
		return "", fmt.Errorf("choose a number from 1 to %d", len(c.models))
	}
	return "", fmt.Errorf("%q is not an available %s model; choose a number or a listed name", terminalText(value), name)
}

// keep reports whether Enter may leave the current model in place.
func (c modelChoices) keep(current string) error {
	if !c.checked || len(c.models) == 0 || current == "" || c.listed(current) {
		return nil
	}
	return fmt.Errorf("%q is not an available %s model; choose a number or a listed name", terminalText(current), screen.HarnessName(c.harness))
}

// suggestions completes typed text from the reported identifiers only. Numbers
// and exact names submit directly instead of opening the menu.
func (c modelChoices) suggestions(line string) []string {
	query := strings.ToLower(strings.TrimSpace(line))
	if query == "" {
		return nil
	}
	if _, err := strconv.Atoi(query); err == nil {
		return nil
	}
	var prefix, contains []string
	for _, m := range c.models {
		id := strings.ToLower(m.ID)
		switch {
		case id == query:
			return nil
		case strings.HasPrefix(id, query):
			prefix = append(prefix, m.ID)
		case strings.Contains(id, query):
			contains = append(contains, m.ID)
		}
	}
	return append(prefix, contains...)
}

func readModelAnswer(ctx context.Context, input LineInput, limit int, choices modelChoices) (string, error) {
	if editor, ok := input.(interface {
		ReadChoice(context.Context, int, func(string) []string) (string, error)
	}); ok && len(choices.models) > 0 {
		return editor.ReadChoice(ctx, limit, choices.suggestions)
	}
	return input.ReadLine(ctx, limit)
}
