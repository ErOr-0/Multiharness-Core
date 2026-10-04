package screen

import (
	"strings"

	"github.com/mattn/go-runewidth"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/transport/cli/term"
)

func (v *View) ContentWidth() int {
	if width, tty := term.Size(v.Writer); tty && width > 0 {
		return max(8, width-3)
	}
	if v.Width <= 0 {
		return 76
	}
	return v.Width
}

func (v *View) Paragraph(value string, indent int, style string) string {
	var text strings.Builder
	for _, line := range term.Wrap(value, v.ContentWidth()+2-indent) {
		text.WriteString(strings.Repeat(" ", indent) + v.Paint(line, style) + "\n")
	}
	return text.String()
}

func (v *View) DetailRow(label, value, style string) string {
	const labelWidth = 14
	if v.ContentWidth() < 60 || runewidth.StringWidth(label) >= labelWidth {
		return v.Paragraph(label, 2, style) + v.Paragraph(value, 4, "0")
	}
	var text strings.Builder
	for i, line := range term.Wrap(value, v.ContentWidth()-labelWidth) {
		prefix := strings.Repeat(" ", labelWidth)
		if i == 0 {
			prefix = v.Paint(label, style) + strings.Repeat(" ", max(0, labelWidth-runewidth.StringWidth(label)))
		}
		text.WriteString("  " + prefix + line + "\n")
	}
	return text.String()
}

func (v *View) ReadinessHeader(mode string) error {
	return v.Print("\n" + v.Paragraph("WORKFLOW READINESS · "+strings.ToUpper(mode), 2, "1;36") + "  " + v.Rule() + "\n" + v.Paragraph("AGENTS", 2, "1"))
}

func (v *View) ReadinessAgent(role, agent, model string, status account.Status) error {
	if model == "" {
		model = "CLI default"
		if agent == "OpenCode" {
			model = "model not selected"
		}
	}
	if role != "" {
		role = strings.ToUpper(role[:1]) + role[1:]
	}
	return v.Print(v.DetailRow(role, agent+" · "+model, "1;36") + v.ReadinessStatus(status))
}

func (v *View) ReadinessStatus(status account.Status) string {
	label, style := "✓ READY", "32"
	if !status.Ready {
		label, style = "! NEEDS SETUP", "33"
	}
	return v.DetailRow(label, status.Detail, style)
}

func (v *View) ReadinessSummary(ready bool) error {
	text := "\n  " + v.Rule() + "\n"
	if ready {
		text += v.Paragraph("✓ Setup checks passed", 2, "1;32")
		text += v.Paragraph("Model access and credits are checked when used.", 4, "2")
	} else {
		text += v.Paragraph("! Setup incomplete · Tasks are blocked", 2, "1;33")
		text += v.DetailRow("Next step", "/setup to finish account setup", "1;36")
		text += v.DetailRow("Check again", "/configuration", "36")
		text += v.DetailRow("Change agents", "/config", "36")
	}
	return v.Print(text)
}
