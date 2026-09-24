package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
)

func TestValidationApprovalShowsExactActionAndRequiresConsent(t *testing.T) {
	for _, answer := range []string{"yes", "YES", "no", "", "y"} {
		var out bytes.Buffer
		p := cli.ValidationConfirmation{Input: confirmationInput(func(context.Context) (string, error) { return answer, nil }), Output: &out}
		yes, err := p.ConfirmValidation(t.Context(), "/workspace/project", store.ValidationAction{Executable: "go", Args: []string{"test", "./..."}, Reason: "cache needs write access\x1b[2J"})
		if err != nil || yes != strings.EqualFold(answer, "yes") {
			t.Fatal(yes, err)
		}
		for _, expected := range []string{`Folder: "/workspace/project"`, `"go" "test" "./..."`, "cache needs write access", "outside the agent sandbox", "[yes/No]"} {
			if !strings.Contains(out.String(), expected) {
				t.Fatal(expected, out.String())
			}
		}
		if strings.Contains(out.String(), "\x1b") {
			t.Fatal("terminal escape injected")
		}
	}
}

func TestValidationApprovalInputFailureNeverAuthorizes(t *testing.T) {
	for _, cause := range []error{io.EOF, context.Canceled, errors.New("broken input")} {
		var out bytes.Buffer
		p := cli.ValidationConfirmation{Input: confirmationInput(func(context.Context) (string, error) { return "yes", cause }), Output: &out}
		if yes, _ := p.ConfirmValidation(t.Context(), "/workspace", store.ValidationAction{Executable: "go", Args: []string{}, Reason: "test"}); yes {
			t.Fatal("authorized on failed input")
		}
	}
}
