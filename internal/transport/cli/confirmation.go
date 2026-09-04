package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"multiharness-core/internal/store"
)

// ConfirmationInput abstracts a context-aware human input surface. Production
// supplies a terminal, not piped stdin or model-generated text.
type ConfirmationInput interface {
	ReadConfirmation(context.Context) (string, error)
}

type BillingConfirmation struct {
	Input  ConfirmationInput
	Output io.Writer
}

func (p BillingConfirmation) ConfirmFallback(ctx context.Context, choice store.AgentSwitch) (bool, error) {
	if err := choice.Validate(); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if p.Input == nil || p.Output == nil {
		return false, nil
	}
	access := "read-only planning/review"
	if choice.CanWrite {
		access = "workspace-write implementation and later repairs; partial changes may already exist"
	}
	message := fmt.Sprintf("\n%s credits are exhausted or its provider usage limit was reached during %s.\nContinue with %s for this role for the rest of this run? Model: %q; %s.\nThis sends task/repository context to the alternate provider and may consume its credits. Type yes to continue [yes/No]: ", choice.From, choice.Stage, choice.To, choice.Model, access)
	if n, err := io.WriteString(p.Output, message); err != nil {
		return false, errors.New("billing confirmation output failed")
	} else if n != len(message) {
		return false, io.ErrShortWrite
	}
	answer, err := p.Input.ReadConfirmation(ctx)
	// End the prompt even on EOF/cancellation before progress can resume. This
	// may add a blank line on echoing terminals but never overwrites user input.
	if n, writeErr := io.WriteString(p.Output, "\n"); writeErr != nil || n != 1 {
		return false, errors.New("billing confirmation output failed")
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if err == io.EOF {
		return false, nil
	}
	if err != nil {
		return false, errors.New("billing confirmation input failed")
	}
	return strings.EqualFold(strings.TrimSpace(answer), "yes"), nil
}
