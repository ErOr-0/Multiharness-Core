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

	"golang.org/x/sys/unix"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/transport/cli/screen"
	"multiharness-core/internal/transport/cli/term"
)

// ReadCommand reads the task prompt with completion from suggest. Configuration
// answers, consent and hidden credentials keep the ordinary bounded reader.
func (p *Reader) ReadCommand(ctx context.Context, limit int, suggest func(string) []string) (answer string, err error) {
	return p.readCommand(ctx, limit, false, suggest)
}

// ReadChoice completes a setup answer from an application-supplied list, such
// as the models a selected CLI reports. Failure details stay at the task prompt.
func (p *Reader) ReadChoice(ctx context.Context, limit int, suggest func(string) []string) (string, error) {
	count := p.failureCount
	p.failureCount = 0
	defer func() { p.failureCount = count }()
	return p.readCommand(ctx, limit, false, suggest)
}

func (p *Reader) ReadFailureDetails(ctx context.Context, v *screen.View, failures []activity.Event, count uint64) error {
	oldView, oldEvents, oldCount := p.commandView, p.failures, p.failureCount
	p.commandView, p.failures, p.failureCount = v, failures, count
	defer func() { p.commandView, p.failures, p.failureCount = oldView, oldEvents, oldCount }()
	_, err := p.readCommand(ctx, 0, true, nil)
	return err
}

func (p *Reader) readCommand(ctx context.Context, limit int, detailsOnly bool, suggest func(string) []string) (answer string, err error) {
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
	fd := int(p.file.Fd())
	original, err := unix.IoctlGetTermios(fd, term.GetTermios)
	if err != nil {
		return "", err
	}
	raw := *original
	raw.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHONL | unix.IEXTEN
	raw.Iflag &^= unix.IXON
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err = unix.IoctlSetTermios(fd, term.SetTermios, &raw); err != nil {
		return "", err
	}
	showingFailures := false
	var pager *screen.FailurePager
	mouseEnabled := p.failureCount > 0 && p.commandView != nil
	defer func() {
		restore := unix.IoctlSetTermios(fd, term.SetTermios, original)
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
	lastWidth, _ := term.Size(p.output)
	redraw := func() error {
		width, _ := term.Size(p.output)
		lastWidth = width
		if width <= 0 {
			width = 80
		}
		// Leave one cell at the right edge so the terminal never auto-wraps.
		display, cursorCells := term.Viewport(line, cursor, width-promptCells-1)
		var out strings.Builder
		out.WriteString("\r\x1b[J" + prompt + paint(display, "0"))
		rows := 0
		if !hiddenMenu && len(suggestions) > 0 && width >= 12 {
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
		if suggest != nil {
			suggestions = suggest(string(line))
		}
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
		// the current width without requiring another keystroke.
		if width, _ := term.Size(p.output); !showingFailures && width > 0 && width != lastWidth {
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
				if term.IncompleteKey(escape) {
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
				return "", term.ErrInputTooLong
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
			if err = redraw(); err != nil {
				return "", err
			}
		}
	}
}
