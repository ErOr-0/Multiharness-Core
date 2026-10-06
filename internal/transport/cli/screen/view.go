package screen

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/term"
)

// View writes themed, width-aware text to Writer. Configure sets the colour
// and width fields from the terminal; tests may set them directly.
type View struct {
	Writer    io.Writer
	Color     bool
	TrueColor bool
	Width     int
	harness   string
}

func (v *View) Configure(cfg config.Config, lookup func(string) (string, bool)) {
	v.harness = cfg.Implementer.Harness
	width, tty := term.Size(v.Writer)
	v.Width = 76
	if tty && width > 0 {
		v.Width = max(8, width-3)
	}
	v.Color = term.Colors(cfg.Color, tty, lookup)
	v.TrueColor = term.TrueColor(lookup)

}

func (v *View) Paint(value, code string) string {
	if !v.Color {
		return value
	}
	return term.Paint(value, code, v.TrueColor)
}

func (v *View) Rule() string { return v.Paint(strings.Repeat("─", v.ContentWidth()), "2") }

func (v *View) Welcome(cfg config.Config) error {
	if err := v.Print("\n  " + v.Paint("◆ magent", "1;36") + "  " + v.Paint("YOUR LOCAL CODING AGENT", "2") + "\n\n  " + v.Rule() + "\n"); err != nil {
		return err
	}
	if err := v.Settings(cfg); err != nil {
		return err
	}
	message := "Choose your workspace and complete the prerequisite checks before starting a task."
	return v.Print("\n" + v.Paragraph(message, 2, "1") + v.DetailRow("/configuration", "Check accounts and workflow readiness", "36") + v.DetailRow("/help", "Commands and shortcuts", "36") + v.DetailRow("/quit", "Exit", "36"))
}

func (v *View) Prompt() error {
	return v.Print("\n  " + v.Rule() + "\n  " + v.Paint("❯", "1;36") + " ")
}

func (v *View) Notice(message string, failed bool) error {
	label, color := "✓", "32"
	if failed {
		label, color = "!", "33"
	}
	lines := term.Wrap(message, v.ContentWidth()-2)
	var text strings.Builder
	for i, line := range lines {
		prefix := "    "
		if i == 0 {
			prefix = "  " + v.Paint(label, color) + " "
		}
		text.WriteString(prefix + line + "\n")
	}
	return v.Print(text.String())
}

func (v *View) Settings(cfg config.Config) error {
	model := func(value string) string {
		if value == "" {
			return "CLI default"
		}
		return value
	}
	text := "\n" + v.Paragraph("WORKSPACE", 2, "1") + v.Paragraph(cfg.WorkingDir, 4, "0") + "\n"
	if cfg.Mode == "direct" {
		deadline, name := cfg.DirectTimeout()
		text += v.Paragraph("DIRECT · ONE AGENT", 2, "1;36")
		text += v.DetailRow("Agent", HarnessName(cfg.Implementer.Harness)+" · "+model(cfg.Implementer.Model), "1;36")
		text += v.DetailRow("Deadline", fmt.Sprintf("%s · /set %s DURATION to change", deadline, name), "2")
		text += v.DetailRow("Permissions", PermissionDescription(cfg)+" /permissions to change.", "2")
		text += v.Paragraph("Follow-ups continue this conversation. /new starts fresh.", 4, "2")
		return v.Print(text)
	}
	text += v.Paragraph("TEAM · PLAN → BUILD → REVIEW", 2, "1;36")
	for _, item := range cfg.RoleAgents() {
		role := item.Agent
		effort := "reasoning: " + model(role.Reasoning)
		label := strings.ToUpper(item.Role[:1]) + item.Role[1:]
		text += v.DetailRow(label, HarnessName(role.Harness)+" · "+model(role.Model), "1;36")
		text += v.DetailRow("", fmt.Sprintf("%s · timeout: %s", effort, time.Duration(role.Timeout)), "2")
	}
	checks := fmt.Sprintf("%d validation checks", len(cfg.Validation.Checks))
	if len(cfg.Validation.Checks) == 0 {
		checks = "No validation checks configured"
	}
	text += "\n" + v.DetailRow("Validation", checks, "2")
	text += v.DetailRow("Repairs", fmt.Sprintf("%d repairs allowed", cfg.MaxRepairAttempts), "2")
	text += v.DetailRow("Permissions", PermissionDescription(cfg)+" /permissions to change implementation access.", "2")
	return v.Print(text)
}

func (v *View) Result(output contract.TaskOutput) error {
	message := output.Summary
	if output.Direct != nil && output.Direct.Text != "" && output.Direct.Text != message {
		message += "\n\n" + output.Direct.Text
	}
	if output.Status == contract.TaskStatusAnswered && output.Plan != nil {
		message = output.Plan.Display()
	}
	if output.Failure != nil {
		message += "\n" + output.Failure.Message
	}
	if output.Status == contract.TaskStatusNeedsInput {
		if output.Failure != nil && output.Failure.Code == contract.FailureCodeValidationInput {
			message += "\n\nRetry when ready to authorize validation, or configure validation.checks for this workspace."
		} else if output.Direct != nil {
			message += "\n\nUse /permissions here to change " + HarnessName(v.harness) + " permissions, then retry your task."
		} else if output.Failure != nil && (output.Failure.Stage == contract.WorkflowStageImplementation || output.Failure.Stage == contract.WorkflowStageRepair) {
			message += "\n\nUse /permissions (or /config > 3) to change implementation permissions, then resubmit your original task. Team mode starts a new workflow and inspects the current files."
		} else {
			message += "\n\nThis role is read-only. Configure the selected CLI's permitted read access or revise the task, then retry. /permissions changes implementation access only."
		}
	}
	if output.Repository != nil && output.Repository.RecoveryDirectory != "" {
		message += "\nStarting files saved at: " + output.Repository.RecoveryDirectory + "\nCurrent edits were kept; no automatic rollback was performed."
	}
	color := "33"
	if output.Status == contract.TaskStatusResponded || output.Status == contract.TaskStatusApproved || output.Status == contract.TaskStatusAnswered {
		color = "32"
	}
	label := strings.ToUpper(string(output.Status))
	if output.Status == contract.TaskStatusAnswered && output.Plan != nil && output.Plan.Action == contract.PlanActionPropose {
		label = "PLANNED"
	}
	return v.Print("\n  " + v.Rule() + "\n" + v.Paragraph(label, 2, "1;"+color) + v.resultBody(message))
}

func (v *View) resultBody(message string) string {
	var text strings.Builder
	limit := max(2, v.ContentWidth()-2)
	for _, line := range strings.Split(contract.PlainText(message), "\n") {
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

func (v *View) Help() error {
	var text strings.Builder
	text.WriteString("\n  " + v.Paint("COMMANDS", "1;36") + "\n\n")
	for _, item := range [][2]string{
		{"/config", "Configure your agent; Docker menu includes project, mode and permissions"},
		{"/new", "Start a fresh conversation"},
		{"/plan REQUEST", "Create and save a plan without implementation"},
		{"/plans [SEARCH]", "Find saved plans in this workspace"},
		{"/use PLAN_ID", "Select a saved plan for a later request"},
		{"/history [SEARCH]", "Show saved exchanges in this conversation"},
		{"/set mode direct|team", "Choose one agent or the full team workflow"},
		{"/login PROVIDER", "Sign in to codex, claude, muse or configure jev"},
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
		text.WriteString(v.DetailRow(item[0], item[1], "36"))
	}
	text.WriteString("\n  " + v.Paint("TRY THIS", "1;36") + "\n\n  /set implementer-model provider/model\n  /set max-repair-attempts 3\n  /set color never\n\n  Type / for suggestions; ↑/↓ select and Tab or Enter fills a command. Enter again submits.\n  Command names ignore case. Quotes and OPTION=VALUE work too.\n  In /config, retry a field or use /cancel to discard setup.\n  Each task starts a fresh workflow.\n")
	text.WriteString("\n  " + v.Paint("EDITING", "1;36") + "\n\n")
	for _, item := range [][2]string{
		{"Enter", "Send the task"},
		{"Shift+Enter · Ctrl+J · \\+Enter", "Add a line; pasted text keeps its line breaks"},
		{"↑ / ↓", "Move between lines, or recall earlier input from this session"},
		{"Ctrl+← / → · Alt+B / F", "Move by word · Ctrl+A / E or Home / End for the line"},
		{"Ctrl+W · Alt+Backspace · Alt+D", "Delete the previous path, word or next word"},
		{"Ctrl+K · Ctrl+U", "Delete to the end or start of the line"},
	} {
		text.WriteString(v.DetailRow(item[0], item[1], "36"))
	}
	return v.Print(text.String())
}
