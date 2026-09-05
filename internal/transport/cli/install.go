package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/workflow"
)

// InstallationConfirmation shares the bounded input contract with billing,
// but consent to one action never authorizes the other.
type InstallationConfirmation struct {
	Input  ConfirmationInput
	Output io.Writer
}

func (p InstallationConfirmation) ConfirmInstall(ctx context.Context, request setup.Request) (bool, error) {
	if ctx == nil {
		return false, errors.New("installation confirmation requires context")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if p.Input == nil || p.Output == nil {
		return false, nil
	}
	if request.Tool != "codex" && request.Tool != "opencode" {
		return false, errors.New("unsupported installation request")
	}
	// Quote even our controlled command so control bytes cannot become terminal
	// instructions if another caller violates the setup adapter contract.
	message := fmt.Sprintf(
		"\n%s CLI is missing. Install it now?\nCommand: %q\nThis downloads a pinned package and runs package installation scripts with your user permissions; it may change global npm packages. No sudo is used.\nAfter installation this run stops: sign in/configure the provider, then rerun. Existing work is not rolled back.\nType yes to install [yes/No]: ",
		request.Tool,
		request.Command,
	)
	if n, err := io.WriteString(p.Output, message); err != nil || n != len(message) {
		return false, errors.New("installation confirmation output failed")
	}
	answer, err := p.Input.ReadConfirmation(ctx)
	if n, writeErr := io.WriteString(p.Output, "\n"); writeErr != nil || n != 1 {
		return false, errors.New("installation confirmation output failed")
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("installation confirmation input failed")
	}
	return strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}

func WithProgressInstallation(confirm setup.Confirmation, events workflow.EventSink) setup.Confirmation {
	pauser, ok := events.(interface{ PauseProgress() (func(), error) })
	if confirm == nil || !ok {
		return confirm
	}
	return func(ctx context.Context, request setup.Request) (bool, error) {
		resume, err := pauser.PauseProgress()
		if resume != nil {
			defer resume()
		}
		if err != nil {
			return false, errors.New("progress output failed before installation confirmation")
		}
		return confirm(ctx, request)
	}
}
