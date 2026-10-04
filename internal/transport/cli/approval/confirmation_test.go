package approval_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/approval"
)

type confirmationInput func(context.Context) (string, error)

func (f confirmationInput) ReadConfirmation(ctx context.Context) (string, error) { return f(ctx) }

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
