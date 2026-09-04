package cli

import (
	"context"
	"errors"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// WithProgressApproval keeps presentation coordination outside workflow policy.
func WithProgressApproval(approver workflow.BillingApprover, events workflow.EventSink) workflow.BillingApprover {
	pauser, ok := events.(interface{ PauseProgress() (func(), error) })
	if approver == nil || !ok {
		return approver
	}
	return &progressApproval{approver: approver, pause: pauser.PauseProgress}
}

type progressApproval struct {
	approver workflow.BillingApprover
	pause    func() (func(), error)
}

func (p *progressApproval) ConfirmFallback(ctx context.Context, choice store.AgentSwitch) (bool, error) {
	resume, err := p.pause()
	if resume != nil {
		defer resume()
	}
	if err != nil {
		return false, errors.New("progress output failed before billing confirmation")
	}
	return p.approver.ConfirmFallback(ctx, choice)
}
