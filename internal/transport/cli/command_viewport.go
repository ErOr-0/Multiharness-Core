package cli

import "github.com/mattn/go-runewidth"

// commandViewport keeps the cursor visible without letting the input wrap into
// the command suggestion menu. Width is measured in terminal cells.
func commandViewport(line []rune, cursor, width int) (string, int) {
	width = max(1, width)
	cursor = max(0, min(cursor, len(line)))
	start, cells := cursor, 0
	for start > 0 {
		cellWidth := max(0, runewidth.RuneWidth(line[start-1]))
		if cells+cellWidth > width {
			break
		}
		start--
		cells += cellWidth
	}
	if start > 0 {
		for start < cursor && cells+1 > width {
			cells -= max(0, runewidth.RuneWidth(line[start]))
			start++
		}
		cells++ // Left ellipsis.
	}
	cursorCells := cells
	end := cursor
	for end < len(line) {
		cellWidth := max(0, runewidth.RuneWidth(line[end]))
		if cells+cellWidth > width {
			break
		}
		end++
		cells += cellWidth
	}
	display := string(line[start:end])
	if start > 0 {
		display = "…" + display
	}
	return display, cursorCells
}
