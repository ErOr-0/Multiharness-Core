package term

import (
	"strings"
	"testing"
)

func TestEditorInsertsTextAndLineBreaks(t *testing.T) {
	var e Editor
	for _, r := range "ab\n界" {
		e.Insert(r)
	}
	if e.Text() != "ab\n界" || e.Cursor() != 4 || e.Bytes() != len("ab\n界") || !e.Multiline() || e.Before() != '界' {
		t.Fatalf("insert: %q cursor=%d bytes=%d", e.Text(), e.Cursor(), e.Bytes())
	}
	e.MoveLineStart()
	if e.Cursor() != 3 {
		t.Fatalf("line start stopped at %d", e.Cursor())
	}
	e.MoveStart()
	e.MoveLineEnd()
	if e.Cursor() != 2 {
		t.Fatalf("line end stopped at %d", e.Cursor())
	}
	e.MoveLeft()
	e.Insert('X')
	if e.Text() != "aXb\n界" || e.Cursor() != 2 {
		t.Fatalf("insert at cursor: %q cursor=%d", e.Text(), e.Cursor())
	}
	e.MoveEnd()
	if !e.DeleteBackward() || e.Text() != "aXb\n" || e.Bytes() != 4 {
		t.Fatalf("delete backward: %q bytes=%d", e.Text(), e.Bytes())
	}
	e.MoveStart()
	if e.DeleteBackward() || !e.DeleteForward() || e.Text() != "Xb\n" {
		t.Fatalf("delete forward: %q", e.Text())
	}
	e.Set("")
	if !e.Empty() || e.Bytes() != 0 || e.Before() != 0 {
		t.Fatal("clear left state behind")
	}
}

func TestEditorWordOperationsFollowReadline(t *testing.T) {
	var e Editor
	e.Set("/set implementer-model foo/bar baz")
	if !e.DeleteBigWordBackward() || e.Text() != "/set implementer-model foo/bar " {
		t.Fatalf("Ctrl+W: %q", e.Text())
	}
	if !e.DeleteBigWordBackward() || e.Text() != "/set implementer-model " {
		t.Fatalf("Ctrl+W over a path: %q", e.Text())
	}
	if !e.DeleteWordBackward() || e.Text() != "/set implementer-" {
		t.Fatalf("Alt+Backspace: %q", e.Text())
	}
	e.MoveStart()
	e.MoveWordRight()
	if e.Cursor() != 4 {
		t.Fatalf("word right stopped at %d", e.Cursor())
	}
	if !e.DeleteWordForward() || e.Text() != "/set-" {
		t.Fatalf("Alt+D: %q", e.Text())
	}
	e.MoveEnd()
	e.MoveWordLeft()
	if e.Cursor() != 1 {
		t.Fatalf("word left stopped at %d", e.Cursor())
	}
	e.MoveEnd()
	if !e.KillToLineStart() || e.Text() != "" || e.KillToLineStart() {
		t.Fatalf("Ctrl+U: %q", e.Text())
	}
}

func TestEditorKillToLineEndRemovesTheBreakWhenAtLineEnd(t *testing.T) {
	var e Editor
	e.Set("one\ntwo")
	e.MoveStart()
	e.MoveLineEnd()
	if !e.KillToLineEnd() || e.Text() != "onetwo" {
		t.Fatalf("Ctrl+K at line end: %q", e.Text())
	}
	if !e.KillToLineEnd() || e.Text() != "one" || e.KillToLineEnd() {
		t.Fatalf("Ctrl+K: %q", e.Text())
	}
}

func TestEditorRowsWrapAndLocateTheCursor(t *testing.T) {
	var e Editor
	e.Set("abcdef\ngh")
	rows, row, col := e.Rows(4)
	if len(rows) != 3 || row != 2 || col != 2 {
		t.Fatalf("rows=%d cursor row=%d col=%d", len(rows), row, col)
	}
	if e.RowText(rows[0]) != "abcd" || !rows[0].continued || e.RowText(rows[1]) != "ef" || rows[1].continued || e.RowText(rows[2]) != "gh" {
		t.Fatalf("wrapped rows: %q %q %q", e.RowText(rows[0]), e.RowText(rows[1]), e.RowText(rows[2]))
	}
	e.MoveStart()
	for range 4 {
		e.MoveRight()
	}
	if _, row, col = e.Rows(4); row != 1 || col != 0 {
		t.Fatalf("cursor after a wrapped row: row=%d col=%d", row, col)
	}
	e.MoveLeft()
	if _, row, col = e.Rows(4); row != 0 || col != 3 {
		t.Fatalf("cursor inside a wrapped row: row=%d col=%d", row, col)
	}
	e.Set("\tx")
	rows, _, col = e.Rows(10)
	if e.RowText(rows[0]) != strings.Repeat(" ", TabCells)+"x" || col != TabCells+1 {
		t.Fatalf("tab row %q col=%d", e.RowText(rows[0]), col)
	}
	e.Set("界界界")
	if rows, row, col = e.Rows(4); len(rows) != 2 || e.RowText(rows[0]) != "界界" || row != 1 || col != 2 {
		t.Fatalf("wide characters: rows=%d row=%d col=%d", len(rows), row, col)
	}
	e.Set("")
	if rows, row, col = e.Rows(1); len(rows) != 1 || row != 0 || col != 0 {
		t.Fatal("empty editor has no row")
	}
}

func TestEditorVerticalMovesKeepTheColumn(t *testing.T) {
	var e Editor
	e.Set("long line here\nab\nlonger line again")
	if !e.MoveUp(40) || e.Cursor() != 17 {
		t.Fatalf("up to a short line: %d", e.Cursor())
	}
	if !e.MoveUp(40) || e.Cursor() != 14 {
		t.Fatalf("up to the first line: %d", e.Cursor())
	}
	if e.MoveUp(40) {
		t.Fatal("moved above the first row")
	}
	if !e.MoveDown(40) || e.Cursor() != 17 || !e.MoveDown(40) || e.Cursor() != e.Len() {
		t.Fatalf("down restored the column: %d", e.Cursor())
	}
	if e.MoveDown(40) {
		t.Fatal("moved below the last row")
	}
	e.MoveStart()
	e.MoveRight()
	if !e.MoveDown(40) || e.Cursor() != 16 {
		t.Fatalf("down keeps the column: %d", e.Cursor())
	}
	e.Set("abcdefgh")
	if !e.MoveUp(4) || e.Cursor() != 3 {
		t.Fatalf("up within a wrapped line: %d", e.Cursor())
	}
	if !e.MoveDown(4) || e.Cursor() != 8 {
		t.Fatalf("down within a wrapped line: %d", e.Cursor())
	}
}
