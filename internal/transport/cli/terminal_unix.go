//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"context"
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"

	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type terminalConfirmation struct {
	file   *os.File
	output io.Writer
}

func terminalSize(writer io.Writer) (int, bool) {
	file, ok := writer.(interface{ Fd() uintptr })
	if !ok {
		return 0, false
	}
	size, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, false
	}
	return int(size.Col), true
}

func NewTerminalApprover(input *os.File, output io.Writer) workflow.BillingApprover {
	return &terminalConfirmation{file: input, output: output}
}

func NewTerminalInstaller(input *os.File, output io.Writer) setup.Confirmation {
	p := &terminalConfirmation{file: input, output: output}
	return func(ctx context.Context, request setup.Request) (bool, error) {
		if !p.available() {
			return false, nil
		}
		return (InstallationConfirmation{Input: p, Output: output}).ConfirmInstall(ctx, request)
	}
}

func (p *terminalConfirmation) ConfirmFallback(ctx context.Context, choice store.AgentSwitch) (bool, error) {
	if !p.available() {
		return false, nil
	}
	return (BillingConfirmation{Input: p, Output: p.output}).ConfirmFallback(ctx, choice)
}

// Consent requires a visible prompt and a human terminal. A character device
// alone is insufficient: /dev/null, redirected output and CI cannot authorize it.
func (p *terminalConfirmation) available() bool {
	if p.file == nil || os.Getenv("CI") != "" {
		return false
	}
	_, inputTerminal := terminalSize(p.file)
	_, outputTerminal := terminalSize(p.output)
	return inputTerminal && outputTerminal
}

func (p *terminalConfirmation) ReadConfirmation(ctx context.Context) (string, error) {
	fd := int(p.file.Fd())
	var line []byte
	tooLong := false
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		_, err := unix.Poll(fds, 100)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return "", err
		}
		if fds[0].Revents == 0 {
			continue
		}
		if fds[0].Revents&unix.POLLIN == 0 {
			return "", io.EOF
		}
		var b [1]byte
		n, err := unix.Read(fd, b[:])
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			return "", err
		}
		if n == 0 {
			return "", io.EOF
		}
		if b[0] == '\n' {
			if tooLong {
				return "", nil
			}
			return string(line), nil
		}
		if len(line) == 64 {
			// Reject this whole response, but drain through newline so its tail
			// cannot become a later prompt's answer or shell input. The normal
			// polling and context checks still bound the wait.
			tooLong = true
		} else {
			line = append(line, b[0])
		}
	}
}
