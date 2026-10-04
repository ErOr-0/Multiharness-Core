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

func TestPermissionRecoveryRequiresExplicitRetry(t *testing.T) {
	for _, answer := range []string{"yes", "YES", "no", "", "y"} {
		var out bytes.Buffer
		p := approval.PermissionRecovery{Input: confirmationInput(func(context.Context) (string, error) { return answer, nil }), Output: &out}
		yes, err := p.ResolvePermission(t.Context(), contract.WorkflowStageImplementation, contract.PermissionDenied{Action: contract.BlockedAction{Tool: "Write", Target: "source.go\x1b[2J"}})
		if err != nil || yes != strings.EqualFold(answer, "yes") {
			t.Fatal(yes, err)
		}
		for _, want := range []string{"implementation", "Write", "source.go", "partial work are retained", "does not grant full access", "[yes/No]"} {
			if !strings.Contains(out.String(), want) {
				t.Fatal(want, out.String())
			}
		}
		if strings.Contains(out.String(), "\x1b") {
			t.Fatal("terminal escape injected")
		}
	}
	for _, cause := range []error{io.EOF, context.Canceled, errors.New("broken input")} {
		p := approval.PermissionRecovery{Input: confirmationInput(func(context.Context) (string, error) { return "yes", cause }), Output: io.Discard}
		if yes, _ := p.ResolvePermission(t.Context(), contract.WorkflowStageReview, contract.PermissionDenied{Action: contract.BlockedAction{Tool: "Read"}}); yes {
			t.Fatal("retried despite input failure")
		}
	}
}
