package approval

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

// PermissionRecovery keeps a blocked team stage alive while the operator fixes
// native access. It does not turn a retry into permission to bypass the harness.
type PermissionRecovery struct {
	Input  ConfirmationInput
	Output io.Writer
}

func (p PermissionRecovery) ResolvePermission(ctx context.Context, stage contract.WorkflowStage, denied contract.PermissionDenied) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := denied.Validate(); err != nil {
		return false, err
	}
	if p.Input == nil || p.Output == nil {
		return false, nil
	}
	message := fmt.Sprintf("\nWorkflow paused during %s\nBlocked tool: %q\nTarget: %q\nThe plan and partial work are retained. Resolve this tool's access in the native harness configuration using another terminal, then retry this stage here. Current sandbox and deny rules still apply; this does not grant full access.\nRetry this stage after resolving access? [yes/No]: ", stage, contract.PlainText(denied.Action.Tool), contract.PlainText(denied.Action.Target))
	if err := writeText(p.Output, message); err != nil {
		return false, errors.New("permission recovery output failed")
	}
	answer, err := p.Input.ReadConfirmation(ctx)
	if writeErr := writeText(p.Output, "\n"); writeErr != nil {
		return false, errors.New("permission recovery output failed")
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if errors.Is(err, io.EOF) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("permission recovery input failed")
	}
	return strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}

type progressPermissionRecovery struct {
	resolver workflow.PermissionResolver
	pause    func() (func(), error)
}

func WithProgressPermissionRecovery(resolver workflow.PermissionResolver, events workflow.EventSink) workflow.PermissionResolver {
	pauser, ok := events.(interface{ PauseProgress() (func(), error) })
	if resolver == nil || !ok {
		return resolver
	}
	return progressPermissionRecovery{resolver, pauser.PauseProgress}
}

func (p progressPermissionRecovery) ResolvePermission(ctx context.Context, stage contract.WorkflowStage, denied contract.PermissionDenied) (bool, error) {
	resume, err := p.pause()
	if resume != nil {
		defer resume()
	}
	if err != nil {
		return false, errors.New("progress output failed before permission recovery")
	}
	return p.resolver.ResolvePermission(ctx, stage, denied)
}
