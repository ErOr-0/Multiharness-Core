package approval_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/approval"
)

type confirmationInput func(context.Context) (string, error)

func (f confirmationInput) ReadConfirmation(ctx context.Context) (string, error) { return f(ctx) }
func switchChoice() contract.AgentSwitch {
	return contract.AgentSwitch{Stage: contract.WorkflowStageImplementation, From: "OpenCode", To: "Codex", Model: "test-model", CanWrite: true}
}

func TestConfirmationAcceptsOnlyExplicitYes(t *testing.T) {
	for _, answer := range []string{"yes", " YES ", "no", "", "y", "yes please", "true"} {
		var prompt bytes.Buffer
		p := approval.BillingConfirmation{Input: confirmationInput(func(context.Context) (string, error) { return answer, nil }), Output: &prompt}
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
		p := approval.BillingConfirmation{Input: confirmationInput(func(context.Context) (string, error) { return "yes", cause }), Output: &prompt}
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
	yes, err := (approval.BillingConfirmation{}).ConfirmFallback(ctx, switchChoice())
	if yes || !errors.Is(err, context.Canceled) {
		t.Fatal("lost cancellation")
	}
}

func TestExistingWorkConfirmationRequiresExplicitYesAndShowsBackup(t *testing.T) {
	request := contract.ExistingWork{WorkingDir: "/project", Files: []string{"app.go"}, RecoveryDirectory: "/state/recovery/one"}
	for _, answer := range []string{"yes", "", "no", "y"} {
		var out bytes.Buffer
		p := approval.WorkspaceConfirmation{Input: confirmationInput(func(context.Context) (string, error) { return answer, nil }), Output: &out}
		yes, err := p.ConfirmExistingWork(t.Context(), request)
		if err != nil || yes != (answer == "yes") || !strings.Contains(out.String(), request.RecoveryDirectory) || !strings.Contains(out.String(), "app.go") {
			t.Fatal("incorrect existing-work consent", err, out.String())
		}
	}
	for _, cause := range []error{io.EOF, context.Canceled} {
		var out bytes.Buffer
		p := approval.WorkspaceConfirmation{Input: confirmationInput(func(context.Context) (string, error) { return "yes", cause }), Output: &out}
		if yes, _ := p.ConfirmExistingWork(t.Context(), request); yes {
			t.Fatal("failed input granted permission")
		}
	}
}
