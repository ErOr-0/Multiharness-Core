package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

type interactiveView struct {
	writer  io.Writer
	color   bool
	width   int
	harness string
}

func (v *interactiveView) configure(cfg config.Config, lookup func(string) (string, bool)) {
	v.harness = cfg.Implementer.Harness
	width, tty := terminalSize(v.writer)
	v.width = 76
	if tty && width > 0 {
		v.width = min(96, max(8, width-3))
	}
	v.color = terminalColors(cfg.Color, tty, lookup)

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
	return "\x1b[" + code + "m" + value + "\x1b[0m"
}

func (v *interactiveView) rule() string { return v.paint(strings.Repeat("─", v.contentWidth()), "2") }

func (v *interactiveView) welcome(cfg config.Config) error {
	if err := interactiveWrite(v.writer, "\n  "+v.paint("◆ magent", "1;36")+"  "+v.paint("YOUR LOCAL CODING AGENT", "2")+"\n\n  "+v.rule()+"\n"); err != nil {
		return err
	}
	if err := v.settings(cfg); err != nil {
		return err
	}
	message := "Choose your workspace and complete the prerequisite checks before starting a task."
	return interactiveWrite(v.writer, "\n"+v.paragraph(message, 2, "1")+v.detailRow("/configuration", "Check accounts and workflow readiness", "36")+v.detailRow("/help", "Commands and shortcuts", "36")+v.detailRow("/quit", "Exit", "36"))
}

func (v *interactiveView) prompt() error {
	return interactiveWrite(v.writer, "\n  "+v.rule()+"\n  "+v.paint("❯", "1;36")+" ")
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
	return interactiveWrite(v.writer, text.String())
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
		return interactiveWrite(v.writer, text)
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
	return interactiveWrite(v.writer, text)
}

func (v *interactiveView) result(output store.TaskOutput) error {
	message := output.Summary
	if output.Direct != nil && output.Direct.Text != "" && output.Direct.Text != message {
		message += "\n\n" + output.Direct.Text
	}
	if output.Status == store.TaskStatusAnswered && output.Plan != nil {
		message = output.Plan.Answer
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
	return interactiveWrite(v.writer, "\n"+v.paint(string(output.Status), "1;"+color)+"\n"+terminalText(message)+"\n")
}

func (v *interactiveView) help() error {
	var text strings.Builder
	text.WriteString("\n  " + v.paint("COMMANDS", "1;36") + "\n\n")
	for _, item := range [][2]string{
		{"/config", "Configure your agent; Docker menu includes project, mode and permissions"},
		{"/new", "Start a fresh direct conversation"},
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
		{"/options", "List all settings and allowed values"},
		{"/quit", "Exit · Ctrl+C also cancels active work"},
	} {
		fmt.Fprintf(&text, "  %s  %s\n", v.paint(fmt.Sprintf("%-19s", item[0]), "36"), item[1])
	}
	text.WriteString("\n  " + v.paint("TRY THIS", "1;36") + "\n\n  /set implementer-model provider/model\n  /set max-repair-attempts 3\n  /set color never\n\n  Type / for suggestions; ↑/↓ select and Tab or Enter fills a command. Enter again submits.\n  Command names ignore case. Quotes and OPTION=VALUE work too.\n  In /config, retry a field or use /cancel to discard setup.\n  Each task starts a fresh workflow.\n")
	return interactiveWrite(v.writer, text.String())
}
