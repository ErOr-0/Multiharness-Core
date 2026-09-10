package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type WorkspaceConfirmation struct {
	Input  ConfirmationInput
	Output io.Writer
}

func (p WorkspaceConfirmation) ConfirmExistingWork(ctx context.Context, request store.ExistingWork) (bool, error) {
	if ctx == nil {
		return false, errors.New("workspace confirmation requires context")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if p.Input == nil || p.Output == nil {
		return false, nil
	}
	if request.WorkingDir == "" || request.RecoveryDirectory == "" || len(request.Files) == 0 {
		return false, errors.New("workspace confirmation requires a concrete backup")
	}
	var text strings.Builder
	fmt.Fprintf(&text, "\nYour folder contains existing work in %d files.\nA backup of the inspected project files is saved at: %s\nFiles excluded by folder ignore rules are not included.\n", len(request.Files), terminalText(request.RecoveryDirectory))
	for _, name := range request.Files[:min(8, len(request.Files))] {
		fmt.Fprintf(&text, "  %q\n", terminalText(name))
	}
	if len(request.Files) > 8 {
		fmt.Fprintf(&text, "  ... and %d more\n", len(request.Files)-8)
	}
	text.WriteString("Allow this task to update these files? Unrelated content must be preserved.\nType yes to continue, or press Enter to stop before edits [yes/No]: ")
	if err := interactiveWrite(p.Output, text.String()); err != nil {
		return false, errors.New("workspace confirmation output failed")
	}
	answer, err := p.Input.ReadConfirmation(ctx)
	if writeErr := interactiveWrite(p.Output, "\n"); writeErr != nil {
		return false, errors.New("workspace confirmation output failed")
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("workspace confirmation input failed")
	}
	return strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}

type progressWorkspaceApproval struct {
	approver workflow.WorkspaceApprover
	pause    func() (func(), error)
}

func WithProgressWorkspaceApproval(approver workflow.WorkspaceApprover, events workflow.EventSink) workflow.WorkspaceApprover {
	pauser, ok := events.(interface{ PauseProgress() (func(), error) })
	if approver == nil || !ok {
		return approver
	}
	return progressWorkspaceApproval{approver, pauser.PauseProgress}
}
func (p progressWorkspaceApproval) ConfirmExistingWork(ctx context.Context, r store.ExistingWork) (bool, error) {
	resume, err := p.pause()
	if resume != nil {
		defer resume()
	}
	if err != nil {
		return false, errors.New("progress output failed before workspace confirmation")
	}
	return p.approver.ConfirmExistingWork(ctx, r)
}
