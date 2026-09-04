package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
)

type confirmationInput func(context.Context) (string, error)

func (f confirmationInput) ReadConfirmation(ctx context.Context) (string, error) { return f(ctx) }
func switchChoice() store.AgentSwitch {
	return store.AgentSwitch{Stage: store.WorkflowStageImplementation, From: "OpenCode", To: "Codex", Model: "test-model", CanWrite: true}
}

func TestConfirmationAcceptsOnlyExplicitYes(t *testing.T) {
	for _, answer := range []string{"yes", " YES ", "no", "", "y", "yes please", "true"} {
		var prompt bytes.Buffer
		p := cli.BillingConfirmation{Input: confirmationInput(func(context.Context) (string, error) { return answer, nil }), Output: &prompt}
		yes, err := p.ConfirmFallback(t.Context(), switchChoice())
		if err != nil || yes != strings.EqualFold(strings.TrimSpace(answer), "yes") {
			t.Fatal("incorrect consent")
		}
		for _, required := range []string{"OpenCode credits", "Codex", "test-model", "partial changes", "consume its credits", "[yes/No]"} {
			if !strings.Contains(prompt.String(), required) {
				t.Fatal("missing informed consent context")
			}
		}
	}
}

func TestConfirmationEOFErrorsAndCancellationFailClosed(t *testing.T) {
	for _, cause := range []error{io.EOF, errors.New("secret input error")} {
		var prompt bytes.Buffer
		p := cli.BillingConfirmation{Input: confirmationInput(func(context.Context) (string, error) { return "yes", cause }), Output: &prompt}
		yes, err := p.ConfirmFallback(t.Context(), switchChoice())
		if yes {
			t.Fatal("accepted failed read")
		}
		if err != nil && strings.Contains(err.Error(), "secret") {
			t.Fatal("diagnostic leak")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	yes, err := (cli.BillingConfirmation{}).ConfirmFallback(ctx, switchChoice())
	if yes || !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation")
	}
}

func TestPipedYesIsNeverConsent(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	_, _ = writer.WriteString("yes\n")
	writer.Close()
	var output bytes.Buffer
	approver := cli.NewTerminalApprover(reader, &output)
	if approver == nil {
		return
	}
	yes, err := approver.ConfirmFallback(t.Context(), switchChoice())
	if err != nil || yes || output.Len() != 0 {
		t.Fatal("non-interactive input authorized fallback")
	}
}
