package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/transport/cli"
)

func TestInstallRequiresExplicitConsent(t *testing.T) {
	for _, answer := range []string{"yes", " YES ", "y", "no", "", "yes please", "true"} {
		t.Run(answer, func(t *testing.T) {
			var output bytes.Buffer
			prompt := cli.InstallationConfirmation{Input: confirmationInput(func(context.Context) (string, error) { return answer, nil }), Output: &output}
			yes, err := prompt.ConfirmInstall(t.Context(), setup.Request{Tool: "opencode", Command: "npm install pinned-package"})
			if err != nil || yes != strings.EqualFold(strings.TrimSpace(answer), "yes") {
				t.Fatal("incorrect consent")
			}
			for _, text := range []string{"opencode", "npm install", "scripts", "global npm packages", "No sudo", "run stops", "[yes/No]"} {
				if !strings.Contains(output.String(), text) {
					t.Fatalf("missing disclosure %q", text)
				}
			}
		})
	}
}

func TestInstallPromptFailsClosed(t *testing.T) {
	for _, mode := range []string{"EOF", "input error", "cancelled", "output error", "short write", "unsupported"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			request := setup.Request{Tool: "codex", Command: "npm install pinned-package"}
			var output bytes.Buffer
			var writer io.Writer = &output
			if mode == "output error" {
				writer = installWriter(func([]byte) (int, error) { return 0, errors.New("SECRET") })
			}
			if mode == "short write" {
				writer = installWriter(func(p []byte) (int, error) { return len(p) - 1, nil })
			}
			if mode == "cancelled" {
				cancel()
			}
			if mode == "unsupported" {
				request.Tool = "untrusted-package"
			}
			prompt := cli.InstallationConfirmation{Input: confirmationInput(func(context.Context) (string, error) {
				if mode == "EOF" {
					return "yes", io.EOF
				}
				return "yes", errors.New("SECRET")
			}), Output: writer}
			yes, err := prompt.ConfirmInstall(ctx, request)
			if yes || (err != nil && strings.Contains(err.Error(), "SECRET")) {
				t.Fatal("unsafe confirmation")
			}
			if mode == "cancelled" && !errors.Is(err, context.Canceled) {
				t.Fatal("lost cancellation")
			}
		})
	}
}

type installWriter func([]byte) (int, error)

func (f installWriter) Write(p []byte) (int, error) { return f(p) }

func TestInstallationNeverAcceptsPipedYes(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	_, _ = writer.WriteString("yes\n")
	writer.Close()
	var output bytes.Buffer
	confirm := cli.NewTerminalInstaller(reader, &output)
	if confirm == nil {
		return
	}
	yes, err := confirm(t.Context(), setup.Request{Tool: "codex", Command: "npm install"})
	if err != nil || yes || output.Len() != 0 {
		t.Fatal("piped input authorized installation")
	}
}
