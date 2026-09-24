package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type ValidationConfirmation struct {
	Input  ConfirmationInput
	Output io.Writer
}

func (p ValidationConfirmation) ConfirmValidation(ctx context.Context, dir string, action store.ValidationAction) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := action.Validate(); err != nil {
		return false, err
	}
	if p.Input == nil || p.Output == nil {
		return false, nil
	}
	parts := []string{strconv.Quote(action.Executable)}
	for _, arg := range action.Args {
		parts = append(parts, strconv.Quote(arg))
	}
	message := fmt.Sprintf("\nValidation needs your approval\nFolder: %q\nCommand (executable and arguments): %s\nReason: %s\nRuns once with the CLI user's permissions, outside the agent sandbox. It may write files and access the network; in Docker it stays inside the container and its mounted folders.\nAllow this command? [yes/No]: ", dir, strings.Join(parts, " "), terminalText(action.Reason))
	if err := interactiveWrite(p.Output, message); err != nil {
		return false, errors.New("validation confirmation output failed")
	}
	answer, err := p.Input.ReadConfirmation(ctx)
	if writeErr := interactiveWrite(p.Output, "\n"); writeErr != nil {
		return false, errors.New("validation confirmation output failed")
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("validation confirmation input failed")
	}
	return strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}

type progressValidationApproval struct {
	approver workflow.ValidationApprover
	pause    func() (func(), error)
}

func WithProgressValidationApproval(approver workflow.ValidationApprover, events workflow.EventSink) workflow.ValidationApprover {
	pauser, ok := events.(interface{ PauseProgress() (func(), error) })
	if approver == nil || !ok {
		return approver
	}
	return progressValidationApproval{approver, pauser.PauseProgress}
}

func (p progressValidationApproval) ConfirmValidation(ctx context.Context, dir string, a store.ValidationAction) (bool, error) {
	resume, err := p.pause()
	if resume != nil {
		defer resume()
	}
	if err != nil {
		return false, errors.New("progress output failed before validation confirmation")
	}
	return p.approver.ConfirmValidation(ctx, dir, a)
}
