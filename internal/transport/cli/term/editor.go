package term

import (
	"slices"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

// TabCells is the fixed display width of a tab inside the editor.
const TabCells = 4

// Editor is a multi-line text buffer with a cursor, both measured in runes.
// It knows nothing about keys or the terminal: the console decodes input
// into these operations and draws the rows that Rows returns.
type Editor struct {
	text   []rune
	cursor int
	bytes  int
	// goal is the preferred cell column kept across consecutive vertical
	// moves, so passing a short line does not lose the column; -1 when unset.
	goal int
}

// Row is one display row: text[Start:End] without its trailing line break.
type Row struct {
	Start, End int
	Cells      int
	// continued marks a soft wrap: the logical line goes on in the next row.
	continued bool
}

func (e *Editor) Text() string    { return string(e.text) }
func (e *Editor) Cursor() int     { return e.cursor }
func (e *Editor) Len() int        { return len(e.text) }
func (e *Editor) Bytes() int      { return e.bytes }
func (e *Editor) Empty() bool     { return len(e.text) == 0 }
func (e *Editor) Multiline() bool { return slices.Contains(e.text, '\n') }

// Before reports the rune immediately before the cursor, or 0 at the start.
func (e *Editor) Before() rune {
	if e.cursor == 0 {
		return 0
	}
	return e.text[e.cursor-1]
}

// Set replaces the text and moves the cursor to its end.
func (e *Editor) Set(text string) {
	e.text = []rune(text)
	e.cursor = len(e.text)
	e.bytes = len(text)
	e.goal = -1
}

// Insert places r at the cursor. Callers keep control characters other than
// line breaks and tabs out, and check the byte limit against Bytes first.
func (e *Editor) Insert(r rune) {
	e.text = slices.Insert(e.text, e.cursor, r)
	e.cursor++
	e.bytes += len(string(r))
	e.goal = -1
}

func (e *Editor) remove(start, end int) bool {
	start, end = max(0, start), min(len(e.text), end)
	if start >= end {
		return false
	}
	e.bytes -= len(string(e.text[start:end]))
	e.text = slices.Delete(e.text, start, end)
	e.cursor = start
	e.goal = -1
	return true
}

func (e *Editor) DeleteBackward() bool { return e.remove(e.cursor-1, e.cursor) }
func (e *Editor) DeleteForward() bool  { return e.remove(e.cursor, e.cursor+1) }

// DeleteWordBackward removes to the start of the current or previous word,
// where words are runs of letters, digits and underscores (Alt+Backspace).
func (e *Editor) DeleteWordBackward() bool { return e.remove(e.wordStart(), e.cursor) }

// DeleteBigWordBackward removes to the previous whitespace boundary like the
// shell's Ctrl+W, so a path or model ID goes in one step.
func (e *Editor) DeleteBigWordBackward() bool { return e.remove(e.bigWordStart(), e.cursor) }

func (e *Editor) DeleteWordForward() bool { return e.remove(e.cursor, e.wordEnd()) }

// KillToLineEnd removes the rest of the line, or the line break itself when
// the cursor already sits at the end of the line.
func (e *Editor) KillToLineEnd() bool {
	end := e.lineEnd()
	if end == e.cursor && end < len(e.text) {
		end++
	}
	return e.remove(e.cursor, end)
}

func (e *Editor) KillToLineStart() bool { return e.remove(e.lineStart(), e.cursor) }

func (e *Editor) move(to int) {
	e.cursor = max(0, min(len(e.text), to))
	e.goal = -1
}

func (e *Editor) MoveLeft()      { e.move(e.cursor - 1) }
func (e *Editor) MoveRight()     { e.move(e.cursor + 1) }
func (e *Editor) MoveWordLeft()  { e.move(e.wordStart()) }
func (e *Editor) MoveWordRight() { e.move(e.wordEnd()) }
func (e *Editor) MoveLineStart() { e.move(e.lineStart()) }
func (e *Editor) MoveLineEnd()   { e.move(e.lineEnd()) }
func (e *Editor) MoveStart()     { e.move(0) }
func (e *Editor) MoveEnd()       { e.move(len(e.text)) }

func (e *Editor) lineStart() int {
	i := e.cursor
	for i > 0 && e.text[i-1] != '\n' {
		i--
	}
	return i
}

func (e *Editor) lineEnd() int {
	i := e.cursor
	for i < len(e.text) && e.text[i] != '\n' {
		i++
	}
	return i
}

func isWord(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func (e *Editor) wordStart() int {
	i := e.cursor
	for i > 0 && !isWord(e.text[i-1]) {
		i--
	}
	for i > 0 && isWord(e.text[i-1]) {
		i--
	}
	return i
}

func (e *Editor) wordEnd() int {
	i := e.cursor
	for i < len(e.text) && !isWord(e.text[i]) {
		i++
	}
	for i < len(e.text) && isWord(e.text[i]) {
		i++
	}
	return i
}

func (e *Editor) bigWordStart() int {
	i := e.cursor
	for i > 0 && unicode.IsSpace(e.text[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(e.text[i-1]) {
		i--
	}
	return i
}

func cellWidth(r rune) int {
	if r == '\t' {
		return TabCells
	}
	return max(0, runewidth.RuneWidth(r))
}

// Rows wraps the text at width cells, breaking long lines between
// characters, and reports the row and cell column holding the cursor.
func (e *Editor) Rows(width int) (rows []Row, row, col int) {
	width = max(1, width)
	current := Row{}
	cells := 0
	for i, r := range e.text {
		if r == '\n' {
			current.End, current.Cells = i, cells
			rows = append(rows, current)
			current, cells = Row{Start: i + 1}, 0
			continue
		}
		w := cellWidth(r)
		if cells+w > width && cells > 0 {
			current.End, current.Cells, current.continued = i, cells, true
			rows = append(rows, current)
			current, cells = Row{Start: i}, 0
		}
		cells += w
	}
	current.End, current.Cells = len(e.text), cells
	rows = append(rows, current)
	for i, r := range rows {
		// After the last character of a wrapped row the cursor shows at the
		// start of the next row; after a line break it stays on its own row.
		if e.cursor < r.End || (e.cursor == r.End && !r.continued) {
			return rows, i, e.cells(r.Start, e.cursor)
		}
	}
	return rows, len(rows) - 1, current.Cells
}

func (e *Editor) cells(start, end int) int {
	total := 0
	for _, r := range e.text[start:end] {
		total += cellWidth(r)
	}
	return total
}

// RowText renders one row with tabs expanded to TabCells spaces.
func (e *Editor) RowText(r Row) string {
	var text strings.Builder
	for _, c := range e.text[r.Start:r.End] {
		if c == '\t' {
			text.WriteString(strings.Repeat(" ", TabCells))
		} else {
			text.WriteRune(c)
		}
	}
	return text.String()
}

// MoveUp moves to the row above, keeping the column where possible. It
// reports false on the first row so the caller can recall history instead.
func (e *Editor) MoveUp(width int) bool { return e.vertical(width, -1) }

// MoveDown is the counterpart of MoveUp and reports false on the last row.
func (e *Editor) MoveDown(width int) bool { return e.vertical(width, 1) }

func (e *Editor) vertical(width, delta int) bool {
	rows, row, col := e.Rows(width)
	target := row + delta
	if target < 0 || target >= len(rows) {
		return false
	}
	if e.goal < 0 {
		e.goal = col
	}
	e.cursor = e.indexAt(rows[target], e.goal)
	return true
}

// indexAt finds the rune in r shown at or before cell column goal.
func (e *Editor) indexAt(r Row, goal int) int {
	cells := 0
	for i := r.Start; i < r.End; i++ {
		w := cellWidth(e.text[i])
		if cells+w > goal {
			return i
		}
		cells += w
	}
	if r.continued && r.End > r.Start {
		return r.End - 1
	}
	return r.End
}
