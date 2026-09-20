package cli

import (
	"regexp"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Keep application colors independent of the host terminal's configurable ANSI
// palette. Styles are scoped to each write; never change the terminal's palette,
// default background, font, or scrollback. Plain output remains escape-free.
func terminalBase(rgb bool) string {
	if rgb {
		return "\x1b[48;2;24;26;32m\x1b[38;2;226;229;237m"
	}
	return "\x1b[48;5;234m\x1b[38;5;254m"
}

// Docker and Apple Terminal commonly advertise 256 colors without truecolor.
// Use explicit RGB only when the terminal declares that capability.
func terminalTrueColor(lookup func(string) (string, bool)) bool {
	if lookup == nil {
		return false
	}
	value, _ := lookup("COLORTERM")
	return strings.EqualFold(value, "truecolor") || strings.EqualFold(value, "24bit")
}

var terminalSGR = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func themePaint(value, style string, trueColor bool) string {
	bold := ""
	if strings.HasPrefix(style, "1;") || style == "1" {
		bold = "1;"
	}
	rgb, indexed := "226;229;237", "254"
	switch strings.TrimPrefix(style, "1;") {
	case "36", "34":
		rgb, indexed = "115;218;242", "117"
	case "32":
		rgb, indexed = "143;223;157", "114"
	case "33":
		rgb, indexed = "255;203;107", "221"
	case "31":
		rgb, indexed = "255;117;127", "210"
	case "2":
		rgb, indexed = "160;170;185", "248"
	}
	if trueColor {
		return "\x1b[" + bold + "38;2;" + rgb + "m" + value + "\x1b[0m"
	}
	return "\x1b[" + bold + "38;5;" + indexed + "m" + value + "\x1b[0m"
}

func (v *interactiveView) write(value string) error {
	return interactiveWrite(v.writer, v.styledText(value))
}

func (v *interactiveView) styledText(value string) string {
	if !v.color {
		return value
	}
	var out strings.Builder
	for len(value) > 0 {
		line, rest, newline := strings.Cut(value, "\n")
		out.WriteString(terminalBase(v.trueColor))
		out.WriteString(strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+terminalBase(v.trueColor)))
		if newline {
			width := runewidth.StringWidth(terminalSGR.ReplaceAllString(line, ""))
			out.WriteString(strings.Repeat(" ", max(0, v.contentWidth()+2-width)))
		}
		out.WriteString("\x1b[0m")
		if newline {
			out.WriteByte('\n')
		}
		value = rest
	}
	return out.String()
}
