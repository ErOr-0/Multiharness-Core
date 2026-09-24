//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type terminalConfirmation struct {
	file           *os.File
	output         io.Writer
	commandView    *interactiveView
	failures       []activity.Event
	failureCount   uint64
	progressCancel context.CancelFunc
	progressDone   chan struct{}
	progressMu     sync.Mutex
}

func NewTerminalValidationApprover(input *os.File, output io.Writer) workflow.ValidationApprover {
	p := &terminalConfirmation{file: input, output: output}
	if !p.available() {
		return nil
	}
	return ValidationConfirmation{Input: p, Output: output}
}

func (p *terminalConfirmation) setCommandView(view *interactiveView) { p.commandView = view }
func (p *terminalConfirmation) setFailures(events []activity.Event, count uint64) {
	p.failures, p.failureCount = append([]activity.Event(nil), events...), count
}

func terminalSize(writer io.Writer) (int, bool) {
	width, _, ok := terminalDimensions(writer)
	return width, ok
}

func terminalDimensions(writer io.Writer) (int, int, bool) {
	file, ok := writer.(interface{ Fd() uintptr })
	if !ok {
		return 0, 0, false
	}
	size, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, false
	}
	return int(size.Col), int(size.Row), true
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
	line, err := p.ReadLine(ctx, 64)
	if errors.Is(err, errInputTooLong) {
		return "", nil
	}
	return line, err
}

func NewTerminalInput(input *os.File, output io.Writer) (LineInput, error) {
	p := &terminalConfirmation{file: input, output: output}
	if !p.available() {
		return nil, errors.New("magent requires an interactive terminal; use --task for scripted runs")
	}
	return p, nil
}

func (p *terminalConfirmation) ReadLine(ctx context.Context, limit int) (string, error) {
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
				return "", errInputTooLong
			}
			return string(line), nil
		}
		if len(line) == limit {
			// Reject this whole response, but drain through newline so its tail
			// cannot become a later prompt's answer or shell input. The normal
			// polling and context checks still bound the wait.
			tooLong = true
		} else {
			line = append(line, b[0])
		}
	}
}

func NewTerminalWorkspaceApprover(input *os.File, output io.Writer) workflow.WorkspaceApprover {
	p := &terminalConfirmation{file: input, output: output}
	if !p.available() {
		return nil
	}
	return p
}
func (p *terminalConfirmation) ConfirmExistingWork(ctx context.Context, r store.ExistingWork) (bool, error) {
	if !p.available() {
		return false, nil
	}
	return (WorkspaceConfirmation{Input: p, Output: p.output}).ConfirmExistingWork(ctx, r)
}

// NewTerminalDecisionKeyPrompt reads a bounded key with terminal echo disabled.
// Restoration runs on success, EOF and cancellation; nothing is persisted.
func NewTerminalDecisionKeyPrompt(input *os.File, output io.Writer) func(context.Context) (string, error) {
	p := &terminalConfirmation{file: input, output: output}
	return func(ctx context.Context) (key string, err error) {
		if !p.available() {
			return "", errors.New("API key input requires an interactive terminal")
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		fd := int(input.Fd())
		original, err := unix.IoctlGetTermios(fd, secretGetTermios)
		if err != nil {
			return "", errors.New("cannot read terminal settings")
		}
		hidden := *original
		hidden.Lflag &^= unix.ECHO | unix.ECHONL
		if err := unix.IoctlSetTermios(fd, secretSetTermios, &hidden); err != nil {
			return "", errors.New("cannot hide API key input")
		}
		defer func() {
			restoreErr := unix.IoctlSetTermios(fd, secretSetTermios, original)
			_, writeErr := io.WriteString(output, "\n")
			if restoreErr != nil || writeErr != nil {
				key, err = "", errors.New("cannot restore terminal after API key input")
			}
		}()
		message := "\nJev is enabled and needs your OpenRouter API key.\nEnter key (hidden; kept only for this session), or Enter to cancel: "
		if n, err := io.WriteString(output, message); err != nil || n != len(message) {
			return "", errors.New("cannot display API key prompt")
		}
		return p.ReadLine(ctx, 512)
	}
}
