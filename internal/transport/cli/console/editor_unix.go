//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package console

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"golang.org/x/sys/unix"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/transport/cli/screen"
	"multiharness-core/internal/transport/cli/term"
)

// editorOptions enables the task-prompt behaviours that setup answers do not
// get: line breaks in typed and pasted text, and ↑/↓ recall of this session's
// earlier submissions.
type editorOptions struct {
	multiline bool
	history   bool
}

// maxHistory bounds the submissions remembered for recall in one session.
// Nothing is persisted.
const maxHistory = 200

// ReadCommand reads the task prompt with completion from suggest. Enter sends
// the text; Shift+Enter, Ctrl+J, Alt+Enter or a backslash before Enter add a
// line, and pasted text keeps its line breaks. Configuration answers, consent
// and hidden credentials keep the ordinary bounded reader.
func (p *Reader) ReadCommand(ctx context.Context, limit int, suggest func(string) []string) (answer string, err error) {
	return p.readCommand(ctx, limit, false, suggest, editorOptions{multiline: true, history: true})
}

// ReadChoice completes a setup answer from an application-supplied list, such
// as the models a selected CLI reports. It stays on one line and never recalls
// task history. Failure details stay at the task prompt.
func (p *Reader) ReadChoice(ctx context.Context, limit int, suggest func(string) []string) (string, error) {
	count := p.failureCount
	p.failureCount = 0
	defer func() { p.failureCount = count }()
	return p.readCommand(ctx, limit, false, suggest, editorOptions{})
}

func (p *Reader) ReadFailureDetails(ctx context.Context, v *screen.View, failures []activity.Event, count uint64) error {
	oldView, oldEvents, oldCount := p.commandView, p.failures, p.failureCount
	p.commandView, p.failures, p.failureCount = v, failures, count
	defer func() { p.commandView, p.failures, p.failureCount = oldView, oldEvents, oldCount }()
	_, err := p.readCommand(ctx, 0, true, nil, editorOptions{})
	return err
}

func (p *Reader) readCommand(ctx context.Context, limit int, detailsOnly bool, suggest func(string) []string, opts editorOptions) (answer string, err error) {
	if os.Getenv("TERM") == "dumb" {
		return p.ReadLine(ctx, limit)
	}
	paint := func(text, style string) string {
		if p.commandView != nil {
			return p.commandView.Paint(text, style)
		}
		return text
	}
	prompt := "  " + paint("❯", "1;36") + " "
	promptCells := 4
	if p.failureCount > 0 {
		prompt = "  " + paint("▶", "1;33") + " " + paint("❯", "1;36") + " "
		promptCells = 6
	}
	indent := strings.Repeat(" ", promptCells)
	fd := int(p.file.Fd())
	original, err := unix.IoctlGetTermios(fd, term.GetTermios)
	if err != nil {
		return "", err
	}
	raw := *original
	raw.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHONL | unix.IEXTEN
	// Without ICRNL, Enter arrives as CR and Ctrl+J as LF, so one can send the
	// text while the other breaks the line.
	raw.Iflag &^= unix.IXON | unix.ICRNL
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err = unix.IoctlSetTermios(fd, term.SetTermios, &raw); err != nil {
		return "", err
	}
	showingFailures := false
	var pager *screen.FailurePager
	mouseEnabled := p.failureCount > 0 && p.commandView != nil
	// The input can span several rows. cursorRow is the terminal cursor's row
	// within it, so every repaint and the final cleanup start from its top.
	cursorRow := 0
	defer func() {
		restore := unix.IoctlSetTermios(fd, term.SetTermios, original)
		cleanup := ""
		if showingFailures {
			cleanup += "\x1b[?1049l"
		}
		if mouseEnabled {
			cleanup += "\x1b[?1000l\x1b[?1006l"
		}
		if cursorRow > 0 {
			cleanup += fmt.Sprintf("\x1b[%dA", cursorRow)
		}
		_, write := io.WriteString(p.output, cleanup+"\x1b[?2004l\r\x1b[J\n")
		if restore != nil || write != nil {
			answer = ""
			err = errors.New("cannot restore command terminal")
		}
	}()
	controls := "\x1b[?2004h"
	if mouseEnabled {
		controls += "\x1b[?1006h\x1b[?1000h"
	}
	if _, err = io.WriteString(p.output, controls); err != nil {
		return "", err
	}
	var editor term.Editor
	selected := 0
	suggestions := []string(nil)
	var pending []byte
	escape := ""
	pasted, pasteCR, overflow := false, false, false
	hiddenMenu := false
	top := 0 // First visible row while the input is taller than the terminal.
	historyIndex := len(p.history)
	draft, draftOverflow := "", false
	lastWidth, lastHeight, _ := term.Dimensions(p.output)
	editWidth := func(width int) int {
		if width <= 0 {
			width = 80
		}
		// Leave one cell at the right edge so the terminal never auto-wraps.
		return max(1, width-promptCells-1)
	}
	writeRows := func(out *strings.Builder, rows []term.Row, first int) {
		for i, row := range rows {
			if i > 0 {
				out.WriteString("\r\n")
			}
			if first+i == 0 {
				out.WriteString(prompt)
			} else {
				out.WriteString(indent)
			}
			out.WriteString(paint(editor.RowText(row), "0"))
		}
	}
	redraw := func() error {
		width, height, _ := term.Dimensions(p.output)
		lastWidth, lastHeight = width, height
		if width <= 0 {
			width = 80
		}
		if height <= 0 {
			height = 24
		}
		rows, at, col := editor.Rows(editWidth(width))
		var menu []string
		if !hiddenMenu && len(suggestions) > 0 && width >= 12 {
			first := max(0, selected-4)
			for i := first; i < min(len(suggestions), first+5); i++ {
				marker, style := "  ", "2"
				if i == selected {
					marker, style = "> ", "1;36"
				}
				value := suggestions[i]
				if len(value) > width-7 {
					value = value[:width-7]
				}
				menu = append(menu, "    "+paint(marker+value, style))
			}
			if width >= 45 {
				menu = append(menu, paint("  ↑/↓ select · Tab/Enter fill · Esc close", "2"))
			}
		}
		hintRows := 0
		if opts.multiline && len(rows) > 1 {
			hintRows = 1
		}
		// Keep the whole drawn input on screen so relative cursor moves stay exact.
		limitRows := max(1, height-len(menu)-hintRows-1)
		if len(rows) <= limitRows {
			top = 0
		} else {
			top = min(top, len(rows)-limitRows)
			if at < top {
				top = at
			}
			if at >= top+limitRows {
				top = at - limitRows + 1
			}
		}
		visible := rows[top:min(len(rows), top+limitRows)]
		var out strings.Builder
		if cursorRow > 0 {
			fmt.Fprintf(&out, "\x1b[%dA", cursorRow)
		}
		out.WriteString("\r\x1b[J")
		writeRows(&out, visible, top)
		below := 0
		if hintRows > 0 {
			if hint := editorHint(width, top, len(rows)-top-len(visible)); hint != "" {
				out.WriteString("\r\n" + paint(hint, "2"))
				below++
			}
		}
		for _, line := range menu {
			out.WriteString("\r\n" + line)
			below++
		}
		// Return to the cursor's row and place it using terminal-cell width.
		if up := len(visible) - 1 - (at - top) + below; up > 0 {
			fmt.Fprintf(&out, "\x1b[%dA", up)
		}
		fmt.Fprintf(&out, "\r\x1b[%dC", promptCells+col)
		cursorRow = at - top
		return term.Write(p.output, out.String())
	}
	toggleFailures := func() error {
		if !mouseEnabled {
			return nil
		}
		if showingFailures {
			showingFailures = false
			if err := term.Write(p.output, "\x1b[?1049l"); err != nil {
				return err
			}
			if detailsOnly {
				return nil
			}
			return redraw()
		}
		if err := term.Write(p.output, "\x1b[?1049h"); err != nil {
			return err
		}
		showingFailures = true
		pager = screen.NewFailurePager(p.failures, p.failureCount)
		return pager.Draw(p.commandView)
	}
	pageInput := func(key string) error {
		if pager.Input(key) {
			return toggleFailures()
		}
		return pager.Draw(p.commandView)
	}
	refresh := func() {
		suggestions, selected, hiddenMenu = nil, 0, false
		if suggest != nil && !editor.Multiline() {
			suggestions = suggest(editor.Text())
		}
	}
	insert := func(r rune) {
		if editor.Bytes()+utf8.RuneLen(r) > limit {
			overflow = true
			return
		}
		editor.Insert(r)
	}
	deleted := func(changed bool) {
		if !changed {
			return
		}
		if editor.Empty() {
			overflow, pending = false, nil
		}
		refresh()
	}
	recall := func(delta int) {
		next := historyIndex + delta
		if !opts.history || next < 0 || next > len(p.history) {
			return
		}
		if historyIndex == len(p.history) {
			draft, draftOverflow = editor.Text(), overflow
		}
		historyIndex = next
		if next == len(p.history) {
			editor.Set(draft)
			overflow = draftOverflow
		} else {
			editor.Set(p.history[next])
			overflow = false
		}
		refresh()
	}
	fill := func() bool {
		if hiddenMenu || len(suggestions) == 0 || overflow {
			return false
		}
		editor.Set(suggestions[selected])
		suggestions, hiddenMenu = nil, true
		return true
	}
	submit := func() (string, error) {
		if overflow || editor.Bytes() > limit {
			return "", term.ErrInputTooLong
		}
		if len(pending) > 0 {
			return "", errors.New("invalid UTF-8 input")
		}
		text := editor.Text()
		if opts.history && strings.TrimSpace(text) != "" && (len(p.history) == 0 || p.history[len(p.history)-1] != text) {
			p.history = append(p.history, text)
			if len(p.history) > maxHistory {
				p.history = p.history[len(p.history)-maxHistory:]
			}
		}
		// Keep the submitted input in the transcript while clearing only the
		// transient hint and menu.
		rows, _, _ := editor.Rows(editWidth(lastWidth))
		var out strings.Builder
		if cursorRow > 0 {
			fmt.Fprintf(&out, "\x1b[%dA", cursorRow)
		}
		out.WriteString("\r\x1b[J")
		writeRows(&out, rows, 0)
		out.WriteString("\n")
		cursorRow = 0
		if err := term.Write(p.output, out.String()); err != nil {
			return "", err
		}
		return text, nil
	}
	act := func(key term.Key) (text string, done bool, err error) {
		switch key {
		case term.KeyNone:
			return "", false, nil
		case term.KeySubmit, term.KeyNewline:
			if fill() {
				return "", false, redraw()
			}
			if !opts.multiline || (key == term.KeySubmit && editor.Before() != '\\') {
				text, err = submit()
				return text, true, err
			}
			if key == term.KeySubmit {
				editor.DeleteBackward() // The backslash only escaped Enter.
			}
			insert('\n')
			refresh()
		case term.KeyTab:
			if !fill() {
				return "", false, nil
			}
		case term.KeyEscape:
			hiddenMenu = true
		case term.KeyEOF:
			if editor.Empty() {
				return "", true, io.EOF
			}
			deleted(editor.DeleteForward())
		case term.KeyBackspace:
			deleted(editor.DeleteBackward())
		case term.KeyDelete:
			deleted(editor.DeleteForward())
		case term.KeyDeleteWordBackward:
			deleted(editor.DeleteWordBackward())
		case term.KeyDeleteBigWordBackward:
			deleted(editor.DeleteBigWordBackward())
		case term.KeyDeleteWordForward:
			deleted(editor.DeleteWordForward())
		case term.KeyKillToLineEnd:
			deleted(editor.KillToLineEnd())
		case term.KeyKillToLineStart:
			deleted(editor.KillToLineStart())
		case term.KeyUp:
			if !hiddenMenu && len(suggestions) > 0 {
				selected = (selected + len(suggestions) - 1) % len(suggestions)
			} else if !editor.MoveUp(editWidth(lastWidth)) {
				recall(-1)
			}
		case term.KeyDown:
			if !hiddenMenu && len(suggestions) > 0 {
				selected = (selected + 1) % len(suggestions)
			} else if !editor.MoveDown(editWidth(lastWidth)) {
				recall(1)
			}
		case term.KeyLeft:
			editor.MoveLeft()
		case term.KeyRight:
			editor.MoveRight()
		case term.KeyWordLeft:
			editor.MoveWordLeft()
		case term.KeyWordRight:
			editor.MoveWordRight()
		case term.KeyLineStart:
			editor.MoveLineStart()
		case term.KeyLineEnd:
			editor.MoveLineEnd()
		case term.KeyTextStart:
			editor.MoveStart()
		case term.KeyTextEnd:
			editor.MoveEnd()
		}
		return "", false, redraw()
	}
	if detailsOnly {
		if err = toggleFailures(); err != nil {
			return "", err
		}
	} else if mouseEnabled {
		if err = redraw(); err != nil {
			return "", err
		}
	}
	for {
		if detailsOnly && !showingFailures {
			return "", nil
		}
		if err = ctx.Err(); err != nil {
			return "", err
		}
		// Polling dimensions also works when Docker does not forward SIGWINCH.
		// Repaint before reading the next key, and while idle, so the input uses
		// the current size without requiring another keystroke.
		if width, height, _ := term.Dimensions(p.output); !showingFailures && width > 0 && (width != lastWidth || height != lastHeight) {
			if err = redraw(); err != nil {
				return "", err
			}
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		_, err = unix.Poll(fds, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return "", err
		}
		if fds[0].Revents == 0 {
			if showingFailures {
				w, h, _ := term.Dimensions(p.output)
				if pager.Resized(w, h) {
					if err = pager.Draw(p.commandView); err != nil {
						return "", err
					}
				}
			}
			if escape != "" {
				// A lone Escape has no terminator; resolve it once input pauses
				// and drop any other sequence that was never completed.
				seq := escape
				escape = ""
				if strings.Trim(seq, "\x1b") != "" || pasted {
					continue
				}
				if showingFailures {
					if err = toggleFailures(); err != nil {
						return "", err
					}
					continue
				}
				if _, _, err = act(term.KeyEscape); err != nil {
					return "", err
				}
			}
			continue
		}
		if fds[0].Revents&unix.POLLIN == 0 {
			return "", io.EOF
		}
		var b [1]byte
		n, readErr := unix.Read(fd, b[:])
		if errors.Is(readErr, unix.EINTR) || errors.Is(readErr, unix.EAGAIN) {
			continue
		}
		if readErr != nil {
			return "", readErr
		}
		if n == 0 {
			return "", io.EOF
		}
		ch := b[0]
		if escape != "" || ch == 27 {
			escape += string(ch)
			if term.IncompleteKey(escape) {
				continue
			}
			seq := escape
			escape = ""
			if showingFailures {
				if err = pageInput(seq); err != nil {
					return "", err
				}
				continue
			}
			if strings.HasPrefix(seq, "\x1b[<") {
				var button, x, y int
				var action rune
				if _, scanErr := fmt.Sscanf(seq, "\x1b[<%d;%d;%d%c", &button, &x, &y, &action); scanErr == nil && action == 'M' && button == 0 && x >= 3 && x <= 5 {
					if err = toggleFailures(); err != nil {
						return "", err
					}
				}
				continue
			}
			key := term.DecodeKey(seq)
			switch {
			case key == term.KeyPasteStart:
				pasted, pasteCR, hiddenMenu = true, false, true
			case key == term.KeyPasteEnd:
				pasted = false
				refresh()
				if err = redraw(); err != nil {
					return "", err
				}
			case pasted:
				// Escape sequences inside pasted text are not keys.
			default:
				text, done, actErr := act(key)
				if done || actErr != nil {
					return text, actErr
				}
			}
			continue
		}
		if showingFailures {
			if err = pageInput(string(ch)); err != nil {
				return "", err
			}
			continue
		}
		if pasted {
			// Pasted CR, LF and CRLF all become one line break; tabs stay.
			// Single-line answers receive spaces instead.
			if ch == '\r' || ch == '\n' {
				skip := ch == '\n' && pasteCR
				pasteCR = ch == '\r'
				if !skip {
					if opts.multiline {
						insert('\n')
					} else {
						insert(' ')
					}
				}
				continue
			}
			pasteCR = false
			if ch == '\t' {
				if opts.multiline {
					insert('\t')
				} else {
					insert(' ')
				}
				continue
			}
			if ch < 32 || ch == 127 {
				continue
			}
		} else if ch < 32 || ch == 127 {
			text, done, actErr := act(term.DecodeKey(string(ch)))
			if done || actErr != nil {
				return text, actErr
			}
			continue
		}
		pending = append(pending, ch)
		if !utf8.FullRune(pending) {
			continue
		}
		r, size := utf8.DecodeRune(pending)
		pending = nil
		if r == utf8.RuneError && size == 1 {
			return "", errors.New("invalid UTF-8 input")
		}
		insert(r)
		if !pasted {
			refresh()
			if err = redraw(); err != nil {
				return "", err
			}
		}
	}
}

// editorHint explains the Enter keys once the input spans several rows and
// counts the rows scrolled out of view when it is taller than the terminal.
func editorHint(width, above, below int) string {
	var counts []string
	if above > 0 {
		counts = append(counts, fmt.Sprintf("↑ %d more", above))
	}
	if below > 0 {
		counts = append(counts, fmt.Sprintf("↓ %d more", below))
	}
	for _, keys := range []string{"Enter sends · Shift+Enter, Ctrl+J or \\+Enter adds a line", "Enter sends · Ctrl+J adds a line", ""} {
		items := counts
		if keys != "" {
			items = append(append([]string(nil), counts...), keys)
		}
		if len(items) == 0 {
			return ""
		}
		hint := "  " + strings.Join(items, " · ")
		if runewidth.StringWidth(hint) < width {
			return hint
		}
	}
	return ""
}
