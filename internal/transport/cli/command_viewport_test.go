package cli

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestCommandViewportUsesAvailableWidth(t *testing.T) {
	line := []rune(strings.Repeat("a", 75))
	if display, cursor := commandViewport(line, len(line), 75); display != string(line) || cursor != 75 {
		t.Fatalf("full-width input was clipped: %q, cursor=%d", display, cursor)
	}
	line = append(line, 'b')
	if display, cursor := commandViewport(line, len(line), 75); display != "…"+strings.Repeat("a", 73)+"b" || cursor != 75 {
		t.Fatalf("overflow did not scroll within terminal width: %q, cursor=%d", display, cursor)
	}
}

func TestCommandViewportTracksWideCharactersAndCursor(t *testing.T) {
	line := []rune("ab界cd")
	for _, cursor := range []int{0, 2, 3, len(line)} {
		display, cursorCells := commandViewport(line, cursor, 6)
		if display != string(line) || cursorCells != runewidth.StringWidth(string(line[:cursor])) {
			t.Fatalf("cursor %d: display=%q cells=%d", cursor, display, cursorCells)
		}
	}
	display, cursorCells := commandViewport(line, len(line), 4)
	if display != "…cd" || cursorCells != 3 {
		t.Fatalf("wide input scrolled incorrectly: %q, cursor=%d", display, cursorCells)
	}
}
