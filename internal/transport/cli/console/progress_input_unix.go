//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package console

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"multiharness-core/internal/transport/cli/progress"
	"multiharness-core/internal/transport/cli/term"

	"golang.org/x/sys/unix"
)

// Mouse reporting is enabled only while an interactive compact task is active.
// The reader gives stdin back before any confirmation, setup or key prompt.
func (p *Reader) StartProgress(ctx context.Context, sink *progress.Sink) {
	p.progressMu.Lock()
	defer p.progressMu.Unlock()
	if p.progressDone != nil || !sink.AcceptsFailureKeys() || !p.available() || os.Getenv("TERM") == "dumb" || ctx.Err() != nil {
		return
	}
	fd := int(p.file.Fd())
	original, err := unix.IoctlGetTermios(fd, term.GetTermios)
	if err != nil {
		return
	}
	raw := *original
	raw.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHONL | unix.IEXTEN
	raw.Iflag &^= unix.IXON
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if unix.IoctlSetTermios(fd, term.SetTermios, &raw) != nil {
		return
	}
	sink.ExclusiveOutput(func() { _, err = io.WriteString(p.output, "\x1b[?1006h\x1b[?1000h") })
	if err != nil {
		_ = unix.IoctlSetTermios(fd, term.SetTermios, original)
		return
	}
	readCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	p.progressCancel, p.progressDone = cancel, done
	go func() {
		defer close(done)
		defer func() {
			sink.ExclusiveOutput(func() { _, _ = io.WriteString(p.output, "\x1b[?1000l\x1b[?1006l") })
			_ = unix.IoctlSetTermios(fd, term.SetTermios, original)
		}()
		var escape string
		for readCtx.Err() == nil {
			fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
			_, err := unix.Poll(fds, 100)
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if err != nil || fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
				return
			}
			if fds[0].Revents&unix.POLLIN == 0 {
				if escape == "\x1b" {
					sink.FailurePageInput(escape)
				}
				escape = ""
				continue
			}
			var b [1]byte
			if n, readErr := unix.Read(fd, b[:]); readErr != nil || n != 1 {
				if errors.Is(readErr, unix.EINTR) || errors.Is(readErr, unix.EAGAIN) {
					continue
				}
				return
			}
			if escape != "" || b[0] == 27 {
				escape += string(b[0])
				if term.IncompleteKey(escape) {
					continue
				}
				if sink.FailurePageInput(escape) {
					escape = ""
					continue
				}
				if strings.HasPrefix(escape, "\x1b[<") {
					if b[0] != 'M' && b[0] != 'm' && len(escape) < 48 {
						continue
					}
					var button, x, y int
					var action rune
					if _, scanErr := fmt.Sscanf(escape, "\x1b[<%d;%d;%d%c", &button, &x, &y, &action); scanErr == nil && action == 'M' && button == 0 && x == 3 {
						sink.ToggleFailureModal()
					}
				}
				escape = ""
				continue
			}
			if sink.FailurePageInput(string(b[0])) {
				continue
			}
			if b[0] == 'd' || b[0] == 'D' {
				sink.ToggleFailureModal()
			}
		}
	}()
}

func (p *Reader) StopProgress() {
	p.progressMu.Lock()
	defer p.progressMu.Unlock()
	if p.progressCancel == nil {
		return
	}
	cancel, done := p.progressCancel, p.progressDone
	p.progressCancel, p.progressDone = nil, nil
	cancel()
	<-done
}
