package cli

import (
	"strings"

	"github.com/mattn/go-runewidth"

	"multiharness-core/internal/adapter/account"
)

// Wrap before adding ANSI styles: escape sequences consume no terminal cells.
// Long model IDs and paths also wrap, without losing continuation indentation.
func wrapTerminal(value string, width int) []string {
	width = max(2, width)
	var lines []string
	for _, paragraph := range strings.Split(terminalText(value), "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			if line != "" && runewidth.StringWidth(line)+1+runewidth.StringWidth(word) > width {
				lines = append(lines, line)
				line = ""
			}
			parts := strings.Split(runewidth.Wrap(word, width), "\n")
			for i, part := range parts {
				if i > 0 {
					lines = append(lines, line)
					line = ""
				}
				if line != "" {
					line += " "
				}
				line += part
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func (v *interactiveView) contentWidth() int {
	if width, tty := terminalSize(v.writer); tty && width > 0 {
		return max(8, width-3)
	}
	if v.width <= 0 {
		return 76
	}
	return v.width
}

func (v *interactiveView) paragraph(value string, indent int, style string) string {
	var text strings.Builder
	for _, line := range wrapTerminal(value, v.contentWidth()+2-indent) {
		text.WriteString(strings.Repeat(" ", indent) + v.paint(line, style) + "\n")
	}
	return text.String()
}

func (v *interactiveView) detailRow(label, value, style string) string {
	const labelWidth = 14
	if v.contentWidth() < 60 || runewidth.StringWidth(label) >= labelWidth {
		return v.paragraph(label, 2, style) + v.paragraph(value, 4, "0")
	}
	var text strings.Builder
	for i, line := range wrapTerminal(value, v.contentWidth()-labelWidth) {
		prefix := strings.Repeat(" ", labelWidth)
		if i == 0 {
			prefix = v.paint(label, style) + strings.Repeat(" ", max(0, labelWidth-runewidth.StringWidth(label)))
		}
		text.WriteString("  " + prefix + line + "\n")
	}
	return text.String()
}

func (v *interactiveView) readinessHeader(mode string) error {
	return v.write("\n" + v.paragraph("WORKFLOW READINESS · "+strings.ToUpper(mode), 2, "1;36") + "  " + v.rule() + "\n" + v.paragraph("AGENTS", 2, "1"))
}

func (v *interactiveView) readinessAgent(role, agent, model string, status account.Status) error {
	if model == "" {
		model = "CLI default"
		if agent == "OpenCode" {
			model = "model not selected"
		}
	}
	if role != "" {
		role = strings.ToUpper(role[:1]) + role[1:]
	}
	return v.write(v.detailRow(role, agent+" · "+model, "1;36") + v.readinessStatus(status))
}

func (v *interactiveView) readinessStatus(status account.Status) string {
	label, style := "✓ READY", "32"
	if !status.Ready {
		label, style = "! NEEDS SETUP", "33"
	}
	return v.detailRow(label, status.Detail, style)
}

func (v *interactiveView) readinessSummary(ready bool) error {
	text := "\n  " + v.rule() + "\n"
	if ready {
		text += v.paragraph("✓ Setup checks passed", 2, "1;32")
		text += v.paragraph("Model access and credits are checked when used.", 4, "2")
	} else {
		text += v.paragraph("! Setup incomplete · Tasks are blocked", 2, "1;33")
		text += v.detailRow("Next step", "/setup to finish account setup", "1;36")
		text += v.detailRow("Check again", "/configuration", "36")
		text += v.detailRow("Change agents", "/config", "36")
	}
	return v.write(text)
}
