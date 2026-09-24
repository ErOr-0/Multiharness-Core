//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// Mouse reporting is enabled only while an interactive compact task is active.
// The reader gives stdin back before any confirmation, setup or key prompt.
func (p *terminalConfirmation) startProgress(ctx context.Context, sink *progressSink) {
	p.progressMu.Lock()
	defer p.progressMu.Unlock()
	if p.progressDone != nil || sink.failureCount.Load() == 0 || !p.available() || os.Getenv("TERM") == "dumb" || ctx.Err() != nil {
		return
	}
	sink.mu.Lock()
	pausedOrStopped := sink.view.paused || sink.view.stopped
	sink.mu.Unlock()
	if pausedOrStopped {
		return
	}
	fd := int(p.file.Fd())
	original, err := unix.IoctlGetTermios(fd, secretGetTermios)
	if err != nil {
		return
	}
	raw := *original
	raw.Lflag &^= unix.ICANON | unix.ECHO | unix.ECHONL | unix.IEXTEN
	raw.Iflag &^= unix.IXON
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if unix.IoctlSetTermios(fd, secretSetTermios, &raw) != nil {
		return
	}
	sink.mu.Lock()
	_, err = io.WriteString(p.output, "\x1b[?1006h\x1b[?1000h")
	sink.mu.Unlock()
	if err != nil {
		_ = unix.IoctlSetTermios(fd, secretSetTermios, original)
		return
	}
	readCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	p.progressCancel, p.progressDone = cancel, done
	go func() {
		defer close(done)
		defer func() {
			sink.mu.Lock()
			_, _ = io.WriteString(p.output, "\x1b[?1000l\x1b[?1006l")
			sink.mu.Unlock()
			_ = unix.IoctlSetTermios(fd, secretSetTermios, original)
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
					sink.failurePageInput(escape)
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
				if incompleteTerminalKey(escape) {
					continue
				}
				if sink.failurePageInput(escape) {
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
						sink.toggleFailureModal()
					}
				}
				escape = ""
				continue
			}
			if sink.failurePageInput(string(b[0])) {
				continue
			}
			if b[0] == 'd' || b[0] == 'D' {
				sink.toggleFailureModal()
			}
		}
	}()
}

func (p *terminalConfirmation) stopProgress() {
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
