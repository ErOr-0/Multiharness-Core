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
	v.width = 64
	if tty && width > 0 {
		v.width = min(64, max(12, width-4))
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

func (v *interactiveView) rule() string { return v.paint(strings.Repeat("─", v.width), "2") }

func (v *interactiveView) welcome(cfg config.Config) error {
	if err := interactiveWrite(v.writer, "\n  "+v.paint("◆ magent", "1;36")+"  "+v.paint("YOUR LOCAL CODING AGENT", "2")+"\n\n  "+v.rule()+"\n"); err != nil {
		return err
	}
	if err := v.settings(cfg); err != nil {
		return err
	}
	message := "Choose your workspace when prompted, then type a task."
	return interactiveWrite(v.writer, "\n  "+v.paint(message, "1")+"\n  "+v.paint("/config", "36")+" configure  ·  "+v.paint("/help", "36")+" commands  ·  "+v.paint("/quit", "36")+" exit\n")
}

func (v *interactiveView) prompt() error {
	return interactiveWrite(v.writer, "\n  "+v.rule()+"\n  "+v.paint("❯", "1;36")+" ")
}

func (v *interactiveView) notice(message string, failed bool) error {
	label, color := "✓", "32"
	if failed {
		label, color = "!", "33"
	}
	return interactiveWrite(v.writer, "  "+v.paint(label, color)+" "+terminalText(message)+"\n")
}

func (v *interactiveView) settings(cfg config.Config) error {
	if cfg.Mode == "direct" {
		model := cfg.Implementer.Model
		if model == "" {
			model = "CLI default"
		}
		deadline, name := cfg.DirectTimeout()
		permissions := "  Permissions: " + permissionDescription(cfg) + " /permissions to change.\n"
		return interactiveWrite(v.writer, fmt.Sprintf("\n  WORKSPACE  %s\n  DIRECT     %s - %s\n  Deadline: %s (%s). /set %s DURATION to change it.\n%s  Follow-ups continue this conversation. /new starts fresh.\n", terminalText(cfg.WorkingDir), harnessName(cfg.Implementer.Harness), terminalText(model), deadline, name, name, permissions))
	}
	planner, harness := cfg.Planner.Model, harnessName(cfg.Planner.Harness)
	model := func(value string) string {
		if value == "" {
			return "CLI default"
		}
		return terminalText(value)
	}
	var text strings.Builder
	implementerHarness := harnessName(cfg.Implementer.Harness)
	effort := func(harness, reasoning, variant string) string {
		if harness == "opencode" {
			return "variant: " + model(variant)
		}
		return "reasoning: " + model(reasoning)
	}
	fmt.Fprintf(&text, "\n  %s  %s\n\n", v.paint("WORKSPACE", "2"), terminalText(cfg.WorkingDir))
	for _, role := range []struct {
		label, harness, model, effort string
		timeout                       config.Duration
	}{
		{"PLAN", harness, model(planner), effort(cfg.Planner.Harness, cfg.Planner.Reasoning, cfg.Planner.Variant), cfg.Planner.Timeout},
		{"BUILD", implementerHarness, model(cfg.Implementer.Model), effort(cfg.Implementer.Harness, cfg.Implementer.Reasoning, cfg.Implementer.Variant), cfg.Implementer.Timeout},
		{"REVIEW", harnessName(cfg.Reviewer.Harness), model(cfg.Reviewer.Model), effort(cfg.Reviewer.Harness, cfg.Reviewer.Reasoning, cfg.Reviewer.Variant), cfg.Reviewer.Timeout},
	} {
		fmt.Fprintf(&text, "  %s  %-8s  %s\n", v.paint(fmt.Sprintf("%-7s", role.label), "36"), role.harness, role.model)
		fmt.Fprintf(&text, "           %s · timeout: %s\n", role.effort, time.Duration(role.timeout))
	}
	checks := fmt.Sprintf("%d validation checks", len(cfg.Validation.Checks))
	if len(cfg.Validation.Checks) == 0 {
		checks = "No validation checks configured"
	}
	fmt.Fprintf(&text, "\n  %s  ·  %d repairs allowed\n", v.paint(checks, "2"), cfg.MaxRepairAttempts)
	fmt.Fprintf(&text, "  Permissions (implementation): %s /permissions to change.\n", permissionDescription(cfg))
	return interactiveWrite(v.writer, text.String())
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
		{"/login PROVIDER", "Sign in to codex, opencode or claude"},
		{"/workspace", "Select a folder to work in"},
		{"/settings", "Show the current configuration"},
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
	text.WriteString("\n  " + v.paint("TRY THIS", "1;36") + "\n\n  /set implementer-model provider/model\n  /set max-repair-attempts 3\n  /set color never\n\n  Command names ignore case. Quotes and OPTION=VALUE work too.\n  In /config, retry a field or use /cancel to discard setup.\n  Each task starts a fresh workflow.\n")
	return interactiveWrite(v.writer, text.String())
}
