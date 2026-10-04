package term

import (
	"strings"

	"multiharness-core/internal/contract"

	"github.com/mattn/go-runewidth"
)

// Wrap before adding ANSI styles: escape sequences consume no terminal cells.
// Long model IDs and paths also wrap, without losing continuation indentation.
func Wrap(value string, width int) []string {
	width = max(2, width)
	var lines []string
	for _, paragraph := range strings.Split(contract.PlainText(value), "\n") {
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
