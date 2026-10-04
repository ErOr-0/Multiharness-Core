package approval

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

type NativePermissionPrompt struct {
	Input  ConfirmationInput
	Output io.Writer
}

func (p NativePermissionPrompt) ApproveNative(ctx context.Context, request contract.NativeApproval) (string, error) {
	if p.Input == nil || p.Output == nil {
		return "", nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s needs permission: %s\n%s\n", contract.PlainText(request.Harness), contract.PlainText(request.Action), contract.PlainText(request.Detail))
	for i, c := range request.Choices {
		fmt.Fprintf(&b, "  %d. %s [%s]\n", i+1, contract.PlainText(c.Label), contract.PlainText(c.Scope))
		if c.Rule != "" {
			fmt.Fprintf(&b, "     Native rule: %s\n", contract.PlainText(c.Rule))
		}
	}
	b.WriteString("Choose a number (Enter denies): ")
	if err := writeText(p.Output, b.String()); err != nil {
		return "", err
	}
	for {
		answer, err := p.Input.ReadConfirmation(ctx)
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			return "", errors.New("native permission input failed")
		}
		if err = writeText(p.Output, "\n"); err != nil {
			return "", err
		}
		answer = strings.TrimSpace(answer)
		if answer == "" {
			return "", nil
		}
		n, err := strconv.Atoi(answer)
		if err == nil && n > 0 && n <= len(request.Choices) {
			return request.Choices[n-1].ID, nil
		}
		if err = writeText(p.Output, "Choose a listed number, or Enter to deny: "); err != nil {
			return "", err
		}
	}
}

type progressNativeApproval struct {
	approver contract.NativeApprover
	pause    func() (func(), error)
}

func WithProgressNativeApproval(a contract.NativeApprover, events workflow.EventSink) contract.NativeApprover {
	if a == nil {
		return nil
	}
	p, ok := events.(interface{ PauseProgress() (func(), error) })
	if !ok {
		return a
	}
	return progressNativeApproval{a, p.PauseProgress}
}
func (p progressNativeApproval) ApproveNative(ctx context.Context, r contract.NativeApproval) (string, error) {
	resume, err := p.pause()
	if resume != nil {
		defer resume()
	}
	if err != nil {
		return "", err
	}
	return p.approver.ApproveNative(ctx, r)
}
