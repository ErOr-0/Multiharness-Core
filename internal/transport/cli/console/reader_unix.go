//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package console

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/approval"
	"multiharness-core/internal/transport/cli/screen"
	"multiharness-core/internal/transport/cli/term"
	"multiharness-core/internal/workflow"
)

// Reader is the terminal input surface shared by the task prompt, consent
// prompts and the live failure view, so only one of them reads stdin at a time.
type Reader struct {
	file           *os.File
	output         io.Writer
	commandView    *screen.View
	failures       []activity.Event
	failureCount   uint64
	progressCancel context.CancelFunc
	progressDone   chan struct{}
	progressMu     sync.Mutex
	// history holds this session's submitted task prompts for ↑/↓ recall. It
	// is never written to disk and setup answers are not added.
	history []string
}

func NewNativeApprover(input *os.File, output io.Writer) contract.NativeApprover {
	p := &Reader{file: input, output: output}
	if !p.available() {
		return nil
	}
	return approval.NativePermissionPrompt{Input: p, Output: output}
}

func NewPermissionResolver(input *os.File, output io.Writer) workflow.PermissionResolver {
	p := &Reader{file: input, output: output}
	if !p.available() {
		return nil
	}
	return approval.PermissionRecovery{Input: p, Output: output}
}

func NewValidationApprover(input *os.File, output io.Writer) workflow.ValidationApprover {
	p := &Reader{file: input, output: output}
	if !p.available() {
		return nil
	}
	return approval.ValidationConfirmation{Input: p, Output: output}
}

func (p *Reader) SetCommandView(view *screen.View) { p.commandView = view }
func (p *Reader) SetFailures(events []activity.Event, count uint64) {
	p.failures, p.failureCount = append([]activity.Event(nil), events...), count
}

func NewInstaller(input *os.File, output io.Writer) setup.Confirmation {
	p := &Reader{file: input, output: output}
	return func(ctx context.Context, request setup.Request) (bool, error) {
		if !p.available() {
			return false, nil
		}
		return (approval.InstallationConfirmation{Input: p, Output: output}).ConfirmInstall(ctx, request)
	}
}

// Consent requires a visible prompt and a human terminal. A character device
// alone is insufficient: /dev/null, redirected output and CI cannot authorize it.
func (p *Reader) available() bool {
	if p.file == nil || os.Getenv("CI") != "" {
		return false
	}
	_, inputTerminal := term.Size(p.file)
	_, outputTerminal := term.Size(p.output)
	return inputTerminal && outputTerminal
}

func (p *Reader) ReadConfirmation(ctx context.Context) (string, error) {
	line, err := p.ReadLine(ctx, 64)
	if errors.Is(err, term.ErrInputTooLong) {
		return "", nil
	}
	return line, err
}

// NewReader reads from input without checking that it is a terminal.
func NewReader(input *os.File, output io.Writer) *Reader {
	return &Reader{file: input, output: output}
}

func NewInput(input *os.File, output io.Writer) (*Reader, error) {
	p := NewReader(input, output)
	if !p.available() {
		return nil, errors.New("magent requires an interactive terminal; use --task for scripted runs")
	}
	return p, nil
}

func (p *Reader) ReadLine(ctx context.Context, limit int) (string, error) {
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
				return "", term.ErrInputTooLong
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

func NewWorkspaceApprover(input *os.File, output io.Writer) workflow.WorkspaceApprover {
	p := &Reader{file: input, output: output}
	if !p.available() {
		return nil
	}
	return p
}
func (p *Reader) ConfirmExistingWork(ctx context.Context, r contract.ExistingWork) (bool, error) {
	if !p.available() {
		return false, nil
	}
	return (approval.WorkspaceConfirmation{Input: p, Output: p.output}).ConfirmExistingWork(ctx, r)
}

// NewDecisionKeyPrompt reads a bounded key with terminal echo disabled.
// Restoration runs on success, EOF and cancellation; nothing is persisted.
func NewDecisionKeyPrompt(input *os.File, output io.Writer) func(context.Context) (string, error) {
	p := &Reader{file: input, output: output}
	return func(ctx context.Context) (key string, err error) {
		if !p.available() {
			return "", errors.New("API key input requires an interactive terminal")
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		fd := int(input.Fd())
		original, err := unix.IoctlGetTermios(fd, term.GetTermios)
		if err != nil {
			return "", errors.New("cannot read terminal settings")
		}
		hidden := *original
		hidden.Lflag &^= unix.ECHO | unix.ECHONL
		if err := unix.IoctlSetTermios(fd, term.SetTermios, &hidden); err != nil {
			return "", errors.New("cannot hide API key input")
		}
		defer func() {
			restoreErr := unix.IoctlSetTermios(fd, term.SetTermios, original)
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
