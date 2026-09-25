package cli

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
	"multiharness-core/internal/adapter/agent/activity"
)

// failurePager keeps the controls outside the scrollable provider output. Every
// row is positioned explicitly; writing the last row must never scroll the title
// away. Events are a snapshot so incoming failures cannot move what is being read.
type failurePager struct {
	events                     []activity.Event
	count                      uint64
	selected, offset, pageSize int
	width, height              int
	outputExpanded             bool
	commandExpanded            bool
	overviews                  []string
}

func newFailurePager(events []activity.Event, count uint64) *failurePager {
	p := &failurePager{events: append([]activity.Event(nil), events...), count: count}
	for i := range p.events {
		e := &p.events[i]
		e.Stage = activity.DisplayText(e.Stage)
		e.Command, e.Error = activity.DetailText(e.Command), activity.DetailText(e.Error)
		if !e.Detailed && e.Output == "" {
			e.Output = e.Text
		}
		e.Output = activity.DetailText(e.Output)
		p.overviews = append(p.overviews, failureOverview(*e))
	}
	return p
}

func (p *failurePager) draw(v *interactiveView) error {
	width, height, _ := terminalDimensions(v.writer)
	return interactiveWrite(v.writer, p.render(v, width, height))
}

func (p *failurePager) render(v *interactiveView, width, height int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	p.width, p.height = width, height
	p.pageSize = max(1, height-6)
	cells := max(1, width-1) // Avoid terminal auto-wrap, including the last row.
	var out strings.Builder
	out.WriteString("\x1b[H\x1b[2J")
	row := func(y int, text, style string) {
		if y > height {
			return
		}
		text = runewidth.Truncate(text, cells, "")
		fmt.Fprintf(&out, "\x1b[%d;1H%s", y, v.paint(text, style))
	}
	row(1, "  ▼ Tool failure details", "1;33")
	if len(p.events) == 0 {
		row(2, "  No retained output is available.", "0")
	} else {
		p.selected = min(max(0, p.selected), len(p.events)-1)
		event := p.events[p.selected]
		summary := activity.DisplayText(event.Summary)
		if summary == "" {
			summary = "tool failed"
		}
		agentLabel := string(event.Agent)
		if event.Stage != "" {
			row(1, "  ▼ Tool failure details · "+strings.ReplaceAll(event.Stage, "\n", " "), "1;33")
		}
		row(2, fmt.Sprintf("  %d/%d · %s · %s", p.selected+1, len(p.events), strings.ReplaceAll(summary, "\n", " "), agentLabel), "1;33")
		text, section := p.overviews[p.selected], "Summary"
		outputArrow, commandArrow := "▶", "▶"
		if p.outputExpanded {
			text, section = event.Output, "Output"
			if text == "" {
				text = "No command output was provided by the agent."
			}
			outputArrow = "▼"
		} else if p.commandExpanded {
			text, section = event.Command, "Command"
			if text == "" {
				text = "The agent did not provide a command for this event."
			}
			commandArrow = "▼"
		}
		// Preserve code indentation but wrap long lines by display cells.
		text = strings.ReplaceAll(text, "\t", "    ")
		lines := strings.Split(runewidth.Wrap(text, max(1, cells-2)), "\n")
		if !p.outputExpanded && !p.commandExpanded {
			lines = wrapTerminal(text, max(1, cells-2))
		}
		p.offset = min(max(0, p.offset), max(0, len(lines)-p.pageSize))
		end := min(len(lines), p.offset+p.pageSize)
		label := fmt.Sprintf("  %s lines %d–%d of %d", section, p.offset+1, end, len(lines))
		if p.count > uint64(len(p.events)) {
			label += fmt.Sprintf(" · latest %d of %d events retained", len(p.events), p.count)
		}
		row(3, label, "2")
		row(4, "  "+outputArrow+" Output [o]  "+commandArrow+" Command [c]", "1;36")
		for i, line := range lines[p.offset:end] {
			style := "0"
			if !p.outputExpanded && !p.commandExpanded {
				switch line {
				case "COMMAND", "REPORTED ERROR", "OUTPUT DIAGNOSTICS", "ERROR DETAILS", "OUTPUT":
					style = "1;36"
				}
			}
			row(5+i, "  "+line, style)
		}
	}
	row(max(1, height-1), "  ↑/↓ scroll · PgUp/PgDn · Home/End · ←/→ failure", "2")
	row(height, "  Enter/Esc: close · o: output · c: command", "2")
	return out.String()
}

// input accepts complete key/mouse sequences. Only the visible disclosure arrow
// closes on a mouse click; clicks on code at the same column do nothing.
func (p *failurePager) input(key string) (close bool) {
	switch key {
	case "\r", "\n", "\x1b", "d", "D":
		return true
	case "o", "O":
		p.outputExpanded = !p.outputExpanded
		p.commandExpanded = false
		p.offset = 0
	case "c", "C":
		p.commandExpanded = !p.commandExpanded
		p.outputExpanded = false
		p.offset = 0
	case "\x1b[A":
		p.offset--
	case "\x1b[B":
		p.offset++
	case "\x1b[5~":
		p.offset -= p.pageSize
	case "\x1b[6~", " ":
		p.offset += p.pageSize
	case "\x1b[H", "\x1b[1~", "\x1b[7~":
		p.offset = 0
	case "\x1b[F", "\x1b[4~", "\x1b[8~":
		p.offset = int(^uint(0) >> 1)
	case "\x1b[C", "n":
		p.selected = min(p.selected+1, len(p.events)-1)
		p.offset = 0
		p.outputExpanded = false
		p.commandExpanded = false
	case "\x1b[D", "p":
		p.selected = max(0, p.selected-1)
		p.offset = 0
		p.outputExpanded = false
		p.commandExpanded = false
	default:
		var button, x, y int
		var action rune
		if _, err := fmt.Sscanf(key, "\x1b[<%d;%d;%d%c", &button, &x, &y, &action); err == nil && action == 'M' {
			switch button {
			case 0:
				if x == 3 && y == 4 {
					p.outputExpanded = !p.outputExpanded
					p.commandExpanded = false
					p.offset = 0
					return false
				}
				if x == 17 && y == 4 {
					p.commandExpanded = !p.commandExpanded
					p.outputExpanded = false
					p.offset = 0
					return false
				}
				return x == 3 && y == 1
			case 64:
				p.offset -= 3
			case 65:
				p.offset += 3
			}
		}
	}
	return false
}

func incompleteTerminalKey(key string) bool {
	if key == "\x1b" || key == "\x1b[" {
		return true
	}
	if strings.HasPrefix(key, "\x1b[<") {
		return len(key) < 48 && !strings.HasSuffix(key, "M") && !strings.HasSuffix(key, "m")
	}
	return len(key) < 12 && key[len(key)-1] >= '0' && key[len(key)-1] <= '9'
}
