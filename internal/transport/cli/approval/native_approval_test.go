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

func TestNativePermissionPromptExplicitChoiceAndScopes(t *testing.T) {
	r := contract.NativeApproval{Harness: "Claude", Action: "Write\x1b[2J", Detail: "file: a.go", Choices: []contract.ApprovalChoice{{ID: "once", Label: "Allow once", Scope: "once"}, {ID: "rule", Label: "Save rule", Scope: "localSettings", Rule: "Write(a.go)"}, {ID: "deny", Label: "Deny", Scope: "once"}}}
	for _, answer := range []string{"1", "2", "3", ""} {
		var output bytes.Buffer
		p := approval.NativePermissionPrompt{Input: confirmationInput(func(context.Context) (string, error) { return answer, nil }), Output: &output}
		got, err := p.ApproveNative(t.Context(), r)
		want := map[string]string{"1": "once", "2": "rule", "3": "deny", "": ""}[answer]
		if err != nil || got != want {
			t.Fatal(got, err)
		}
		if strings.Contains(output.String(), "\x1b") || !strings.Contains(output.String(), "localSettings") || !strings.Contains(output.String(), "Write(a.go)") {
			t.Fatal(output.String())
		}
	}
	p := approval.NativePermissionPrompt{Input: confirmationInput(func(context.Context) (string, error) { return "2", io.EOF }), Output: io.Discard}
	if got, err := p.ApproveNative(t.Context(), r); err != nil || got != "" {
		t.Fatal("EOF approved", got, err)
	}
}
