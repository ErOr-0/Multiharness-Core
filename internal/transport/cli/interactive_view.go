package cli

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

type interactiveView struct {
	writer    io.Writer
	color     bool
	trueColor bool
	width     int
	harness   string
}

func (v *interactiveView) configure(cfg config.Config, lookup func(string) (string, bool)) {
	v.harness = cfg.Implementer.Harness
	width, tty := terminalSize(v.writer)
	v.width = 76
	if tty && width > 0 {
		v.width = max(8, width-3)
	}
	v.color = terminalColors(cfg.Color, tty, lookup)
	v.trueColor = terminalTrueColor(lookup)

}

// Shared with lifecycle progress so the shell and running task honor the same
// terminal capabilities and explicit colour preferences.
func terminalColors(mode string, tty bool, lookup func(string) (string, bool)) bool {
	env := func(key string) string {
		if lookup != nil {
			value, _ := lookup(key)
			return value
		}
		return ""
	}
	return mode != "never" && (tty || mode == "always") && env("NO_COLOR") == "" && env("TERM") != "dumb" && (env("CI") == "" || mode == "always")
}

func (v *interactiveView) paint(value, code string) string {
	if !v.color {
		return value
	}
	return themePaint(value, code, v.trueColor)
}

func (v *interactiveView) rule() string { return v.paint(strings.Repeat("─", v.contentWidth()), "2") }

func (v *interactiveView) welcome(cfg config.Config) error {
	if err := v.write("\n  " + v.paint("◆ magent", "1;36") + "  " + v.paint("YOUR LOCAL CODING AGENT", "2") + "\n\n  " + v.rule() + "\n"); err != nil {
		return err
	}
	if err := v.settings(cfg); err != nil {
		return err
	}
	message := "Choose your workspace and complete the prerequisite checks before starting a task."
	return v.write("\n" + v.paragraph(message, 2, "1") + v.detailRow("/configuration", "Check accounts and workflow readiness", "36") + v.detailRow("/help", "Commands and shortcuts", "36") + v.detailRow("/quit", "Exit", "36"))
}

func (v *interactiveView) prompt() error {
	return v.write("\n  " + v.rule() + "\n  " + v.paint("❯", "1;36") + " ")
}

func (v *interactiveView) notice(message string, failed bool) error {
	label, color := "✓", "32"
	if failed {
		label, color = "!", "33"
	}
	lines := wrapTerminal(message, v.contentWidth()-2)
	var text strings.Builder
	for i, line := range lines {
		prefix := "    "
		if i == 0 {
			prefix = "  " + v.paint(label, color) + " "
		}
		text.WriteString(prefix + line + "\n")
	}
	return v.write(text.String())
}

func (v *interactiveView) settings(cfg config.Config) error {
	model := func(value string) string {
		if value == "" {
			return "CLI default"
		}
		return value
	}
	text := "\n" + v.paragraph("WORKSPACE", 2, "1") + v.paragraph(cfg.WorkingDir, 4, "0") + "\n"
	if cfg.Mode == "direct" {
		deadline, name := cfg.DirectTimeout()
		text += v.paragraph("DIRECT · ONE AGENT", 2, "1;36")
		text += v.detailRow("Agent", harnessName(cfg.Implementer.Harness)+" · "+model(cfg.Implementer.Model), "1;36")
		text += v.detailRow("Deadline", fmt.Sprintf("%s · /set %s DURATION to change", deadline, name), "2")
		text += v.detailRow("Permissions", permissionDescription(cfg)+" /permissions to change.", "2")
		text += v.paragraph("Follow-ups continue this conversation. /new starts fresh.", 4, "2")
		return v.write(text)
	}
	text += v.paragraph("TEAM · PLAN → BUILD → REVIEW", 2, "1;36")
	for _, item := range requirements(cfg) {
		role := item.agent
		effort := "reasoning: " + model(role.Reasoning)
		if role.Harness == "opencode" {
			effort = "variant: " + model(role.Variant)
		}
		label := strings.ToUpper(item.role[:1]) + item.role[1:]
		text += v.detailRow(label, harnessName(role.Harness)+" · "+model(role.Model), "1;36")
		text += v.detailRow("", fmt.Sprintf("%s · timeout: %s", effort, time.Duration(role.Timeout)), "2")
	}
	checks := fmt.Sprintf("%d validation checks", len(cfg.Validation.Checks))
	if len(cfg.Validation.Checks) == 0 {
		checks = "No validation checks configured"
	}
	text += "\n" + v.detailRow("Validation", checks, "2")
	text += v.detailRow("Repairs", fmt.Sprintf("%d repairs allowed", cfg.MaxRepairAttempts), "2")
	text += v.detailRow("Permissions", permissionDescription(cfg)+" /permissions to change implementation access.", "2")
	return v.write(text)
}

func (v *interactiveView) result(output store.TaskOutput) error {
	message := output.Summary
	if output.Direct != nil && output.Direct.Text != "" && output.Direct.Text != message {
		message += "\n\n" + output.Direct.Text
	}
	if output.Status == store.TaskStatusAnswered && output.Plan != nil {
		message = output.Plan.Display()
	}
	if output.Failure != nil {
		message += "\n" + output.Failure.Message
	}
	if output.Status == store.TaskStatusNeedsInput {
		if output.Direct != nil {
			message += "\n\nUse /permissions here to change " + harnessName(v.harness) + " permissions, then retry your task."
		} else if output.Failure != nil && (output.Failure.Stage == store.WorkflowStageImplementation || output.Failure.Stage == store.WorkflowStageRepair) {
			message += "\n\nUse /permissions (or /config > 3) to change implementation permissions, then resubmit your original task. Team mode starts a new workflow and inspects the current files."
		} else {
			message += "\n\nThis role is read-only. Configure the selected CLI's permitted read access or revise the task, then retry. /permissions changes implementation access only."
		}
	}
	if output.Repository != nil && output.Repository.RecoveryDirectory != "" {
		message += "\nStarting files saved at: " + output.Repository.RecoveryDirectory + "\nCurrent edits were kept; no automatic rollback was performed."
	}
	color := "33"
	if output.Status == store.TaskStatusResponded || output.Status == store.TaskStatusApproved || output.Status == store.TaskStatusAnswered {
		color = "32"
	}
	label := strings.ToUpper(string(output.Status))
	if output.Status == store.TaskStatusAnswered && output.Plan != nil && output.Plan.Action == store.PlanActionPropose {
		label = "PLANNED"
	}
	return v.write("\n  " + v.rule() + "\n" + v.paragraph(label, 2, "1;"+color) + v.resultBody(message))
}

func (v *interactiveView) resultBody(message string) string {
	var text strings.Builder
	limit := max(2, v.contentWidth()-2)
	for _, line := range strings.Split(terminalText(message), "\n") {
		for _, part := range wrapResultLine(line, limit) {
			text.WriteString("    " + part + "\n")
		}
	}
	return text.String()
}

// Result text can include code and quoted values; wrapping must preserve every
// original space, even when a line exceeds the terminal width.
func wrapResultLine(line string, limit int) []string {
	if line == "" {
		return []string{""}
	}
	var parts []string
	for line != "" {
		cells, cut := 0, 0
		for cut < len(line) {
			r, size := utf8.DecodeRuneInString(line[cut:])
			width := runewidth.RuneWidth(r)
			if r == '\t' {
				width = 8 - ((4 + cells) % 8) // Four display-indent cells precede the text.
			}
			if cells+width > limit && cut > 0 {
				break
			}
			cells += width
			cut += size
		}
		parts = append(parts, line[:cut])
		line = line[cut:]
	}
	return parts
}

func (v *interactiveView) help() error {
	var text strings.Builder
	text.WriteString("\n  " + v.paint("COMMANDS", "1;36") + "\n\n")
	for _, item := range [][2]string{
		{"/config", "Configure your agent; Docker menu includes project, mode and permissions"},
		{"/new", "Start a fresh conversation"},
		{"/plan REQUEST", "Create and save a plan without implementation"},
		{"/plans [SEARCH]", "Find saved plans in this workspace"},
		{"/use PLAN_ID", "Select a saved plan for a later request"},
		{"/history [SEARCH]", "Show saved exchanges in this conversation"},
		{"/set mode direct|team", "Choose one agent or the full team workflow"},
		{"/login PROVIDER", "Sign in to codex, opencode, claude or configure jev"},
		{"/workspace", "Select a folder to work in"},
		{"/setup", "Complete missing account sign-ins and Jev setup"},
		{"/configuration", "Check selected agents, account readiness and Jev setup"},
		{"/settings", "Show the current settings"},
		{"/permissions [MODE]", "Set and save the selected agent's permissions; keep the conversation"},
		{"/set OPTION VALUE", "Change a setting"},
		{"/load PATH", "Load a JSON configuration"},
		{"/save", "Remember your settings"},
		{"/diagnostics", "Show the last saved provider failure"},
		{"/failures", "Expand recent tool failures from the last task"},
		{"/options", "List all settings and allowed values"},
		{"/quit", "Exit · Ctrl+C also cancels active work"},
	} {
		text.WriteString(v.detailRow(item[0], item[1], "36"))
	}
	text.WriteString("\n  " + v.paint("TRY THIS", "1;36") + "\n\n  /set implementer-model provider/model\n  /set max-repair-attempts 3\n  /set color never\n\n  Type / for suggestions; ↑/↓ select and Tab or Enter fills a command. Enter again submits.\n  Command names ignore case. Quotes and OPTION=VALUE work too.\n  In /config, retry a field or use /cancel to discard setup.\n  Each task starts a fresh workflow.\n")
	return v.write(text.String())
}
