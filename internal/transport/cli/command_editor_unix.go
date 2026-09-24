//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"
	"multiharness-core/internal/adapter/agent/activity"
)

// ReadCommand enables completion only at the task prompt. Configuration answers,
// consent and hidden credentials keep the ordinary bounded reader.
func (p *terminalConfirmation) ReadCommand(ctx context.Context, limit int) (answer string, err error) {
	return p.readCommand(ctx, limit, false)
}

func (p *terminalConfirmation) readFailureDetails(ctx context.Context, v *interactiveView, failures []activity.Event, count uint64) error {
	oldView, oldEvents, oldCount := p.commandView, p.failures, p.failureCount
	p.commandView, p.failures, p.failureCount = v, failures, count
	defer func() { p.commandView, p.failures, p.failureCount = oldView, oldEvents, oldCount }()
	_, err := p.readCommand(ctx, 0, true)
	return err
}

func (p *terminalConfirmation) readCommand(ctx context.Context, limit int, detailsOnly bool) (answer string, err error) {
	if os.Getenv("TERM") == "dumb" {
		return p.ReadLine(ctx, limit)
	}
	paint := func(text, style string) string {
		if p.commandView != nil {
			return p.commandView.paint(text, style)
		}
		return text
	}
	prompt := "  " + paint("❯", "1;36") + " "
	promptCells := 4
	if p.failureCount > 0 {
		prompt = "  " + paint("▶", "1;33") + " " + paint("❯", "1;36") + " "
		promptCells = 6
	}
	fd := int(p.file.Fd())
	original, err := unix.IoctlGetTermios(fd, secretGetTermios)
	if err != nil {
		return "", err
	}
	raw := *original
	raw.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHONL | unix.IEXTEN
	raw.Iflag &^= unix.IXON
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err = unix.IoctlSetTermios(fd, secretSetTermios, &raw); err != nil {
		return "", err
	}
	showingFailures := false
	var pager *failurePager
	mouseEnabled := p.failureCount > 0 && p.commandView != nil
	defer func() {
		restore := unix.IoctlSetTermios(fd, secretSetTermios, original)
		cleanup := ""
		if showingFailures {
			cleanup += "\x1b[?1049l"
		}
		if mouseEnabled {
			cleanup += "\x1b[?1000l\x1b[?1006l"
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
	var line []rune
	cursor, selected := 0, 0
	suggestions := []string(nil)
	var pending []byte
	escape := ""
	pasted, overflow := false, false
	hiddenMenu := false
	redraw := func() error {
		width, _ := terminalSize(p.output)
		if width < 12 {
			width = 80
		}
		// Leave one cell at the right edge so the terminal never auto-wraps.
		// Leave one cell to avoid soft wrapping at the right edge.
		display, cursorCells := commandViewport(line, cursor, width-promptCells-1)
		var out strings.Builder
		out.WriteString("\r\x1b[J" + prompt + paint(display, "0"))
		rows := 0
		if !hiddenMenu && len(suggestions) > 0 {
			first := max(0, selected-4)
			for i := first; i < min(len(suggestions), first+5); i++ {
				marker := "  "
				if i == selected {
					marker = "> "
				}
				value := suggestions[i]
				if len(value) > width-7 {
					value = value[:width-7]
				}
				style := "2"
				if i == selected {
					style = "1;36"
				}
				out.WriteString("\r\n    " + paint(marker+value, style))
				rows++
			}
			hint := "  ↑/↓ select · Tab/Enter fill · Esc close"
			if width >= 45 {
				out.WriteString("\r\n" + paint(hint, "2"))
				rows++
			}
		}
		if rows > 0 {
			fmt.Fprintf(&out, "\x1b[%dA", rows)
		}
		// Return to the input row and place the cursor using terminal-cell width.
		fmt.Fprintf(&out, "\r\x1b[%dC", promptCells)
		if cursorCells > 0 {
			fmt.Fprintf(&out, "\x1b[%dC", cursorCells)
		}
		return interactiveWrite(p.output, out.String())
	}
	toggleFailures := func() error {
		if !mouseEnabled {
			return nil
		}
		if showingFailures {
			showingFailures = false
			if err := interactiveWrite(p.output, "\x1b[?1049l"); err != nil {
				return err
			}
			if detailsOnly {
				return nil
			}
			return redraw()
		}
		if err := interactiveWrite(p.output, "\x1b[?1049h"); err != nil {
			return err
		}
		showingFailures = true
		pager = newFailurePager(p.failures, p.failureCount)
		return pager.draw(p.commandView)
	}
	pageInput := func(key string) error {
		if pager.input(key) {
			return toggleFailures()
		}
		return pager.draw(p.commandView)
	}
	refresh := func() { suggestions = CommandSuggestions(string(line)); selected = 0; hiddenMenu = false }
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
				w, h, _ := terminalDimensions(p.output)
				if w > 0 && h > 0 && (w != pager.width || h != pager.height) {
					if err = pager.draw(p.commandView); err != nil {
						return "", err
					}
				}
			}
			if escape == "\x1b" {
				escape = ""
				if showingFailures {
					if err = toggleFailures(); err != nil {
						return "", err
					}
					continue
				}
				hiddenMenu = true
				if err = redraw(); err != nil {
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
			if showingFailures {
				if incompleteTerminalKey(escape) {
					continue
				}
				key := escape
				escape = ""
				if err = pageInput(key); err != nil {
					return "", err
				}
				continue
			}
			if strings.HasPrefix(escape, "\x1b[<") {
				if ch != 'M' && ch != 'm' && len(escape) < 48 {
					continue
				}
				seq := escape
				escape = ""
				var button, x, y int
				var action rune
				if _, scanErr := fmt.Sscanf(seq, "\x1b[<%d;%d;%d%c", &button, &x, &y, &action); scanErr == nil && action == 'M' && button == 0 && x >= 3 && x <= 5 {
					if err = toggleFailures(); err != nil {
						return "", err
					}
				}
				continue
			}
			if escape == "\x1b" || escape == "\x1b[" {
				continue
			}
			if len(escape) < 12 && ch >= '0' && ch <= '9' {
				continue
			}
			seq := escape
			escape = ""
			if seq == "\x1b[200~" {
				pasted = true
				hiddenMenu = true
				continue
			}
			if seq == "\x1b[201~" {
				pasted = false
				refresh()
				if err = redraw(); err != nil {
					return "", err
				}
				continue
			}
			if pasted {
				continue
			}
			switch seq {
			case "\x1b[A":
				if len(suggestions) > 0 {
					selected = (selected + len(suggestions) - 1) % len(suggestions)
					hiddenMenu = false
				}
			case "\x1b[B":
				if len(suggestions) > 0 {
					selected = (selected + 1) % len(suggestions)
					hiddenMenu = false
				}
			case "\x1b[D":
				cursor = max(0, cursor-1)
			case "\x1b[C":
				cursor = min(len(line), cursor+1)
			case "\x1b[H":
				cursor = 0
			case "\x1b[F":
				cursor = len(line)
			case "\x1b[3~":
				if cursor < len(line) {
					line = append(line[:cursor], line[cursor+1:]...)
					refresh()
				}
			}
			if err = redraw(); err != nil {
				return "", err
			}
			continue
		}
		if showingFailures {
			if err = pageInput(string(ch)); err != nil {
				return "", err
			}
			continue
		}
		if !pasted && (ch == '\n' || ch == '\r' || ch == '\t') {
			if !hiddenMenu && len(suggestions) > 0 && !overflow {
				line = []rune(suggestions[selected])
				cursor = len(line)
				suggestions = nil
				hiddenMenu = true
				if err = redraw(); err != nil {
					return "", err
				}
				continue
			}
			if ch == '\t' {
				continue
			}
			if overflow || len(string(line)) > limit {
				return "", errInputTooLong
			}
			if len(pending) > 0 {
				return "", errors.New("invalid UTF-8 input")
			}
			// Preserve submitted input while clearing only the transient menu.
			if _, err = io.WriteString(p.output, "\r\x1b[J"+prompt+paint(string(line), "0")+"\n"); err != nil {
				return "", err
			}
			return string(line), nil
		}
		if !pasted {
			switch ch {
			case 4:
				if len(line) == 0 {
					return "", io.EOF
				}
				continue
			case 127, 8:
				if cursor > 0 {
					line = append(line[:cursor-1], line[cursor:]...)
					cursor--
					refresh()
				}
				if err = redraw(); err != nil {
					return "", err
				}
				continue
			case 21:
				line = nil
				cursor = 0
				overflow = false
				pending = nil
				refresh()
				if err = redraw(); err != nil {
					return "", err
				}
				continue
			case 1:
				cursor = 0
				if err = redraw(); err != nil {
					return "", err
				}
				continue
			case 5:
				cursor = len(line)
				if err = redraw(); err != nil {
					return "", err
				}
				continue
			}
		}
		if pasted && (ch == '\n' || ch == '\r' || ch == '\t') {
			ch = ' '
		}
		if ch < 32 || ch == 127 {
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
		if len(string(line))+size > limit {
			overflow = true
			continue
		}
		line = append(line, 0)
		copy(line[cursor+1:], line[cursor:])
		line[cursor] = r
		cursor++
		if !pasted {
			refresh()
		}
		if err = redraw(); err != nil {
			return "", err
		}
	}
}
