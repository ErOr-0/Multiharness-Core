//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/store"
)

func TestTerminalConsentRequiresVisiblePromptOutsideCI(t *testing.T) {
	// Run this test binary under script's PTY, without a provider or interactive
	// input. No platform-specific ioctl setup or additional Go dependency is needed.
	if os.Getenv("MULTIHARNESS_TERMINAL_TEST") != "1" {
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		run := "-test.run=^TestTerminalConsentRequiresVisiblePromptOutsideCI$"
		var args []string
		switch runtime.GOOS {
		case "darwin":
			args = []string{"-q", "/dev/null", binary, run}
		case "linux":
			command := "exec '" + strings.ReplaceAll(binary, "'", "'\\''") + "' '" + run + "'"
			args = []string{"-q", "-e", "-c", command, "/dev/null"}
		default:
			t.Skip("PTY fixture supports the macOS and Linux test platforms")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		t.Setenv("MULTIHARNESS_TERMINAL_TEST", "1")
		command := exec.CommandContext(ctx, "/usr/bin/script", args...)
		command.WaitDelay = time.Second
		output, err := command.CombinedOutput()
		if err != nil || !bytes.Contains(output, []byte("PASS")) {
			t.Fatalf("terminal fixture: %v\n%s", err, output)
		}
		return
	}
	terminal := os.Stdin
	if _, ok := terminalSize(terminal); !ok {
		t.Fatal("PTY is not recognized as a terminal")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	choice := store.AgentSwitch{Stage: store.WorkflowStagePlanning, From: "Codex", To: "OpenCode", Model: "test/model"}
	for _, test := range []struct {
		name       string
		ci         string
		redirected bool
		wantError  error
	}{
		{name: "visible terminal", wantError: context.Canceled},
		{name: "redirected prompt", redirected: true},
		{name: "CI terminal", ci: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("CI", test.ci)
			var redirected bytes.Buffer
			var output io.Writer = os.Stderr
			if test.redirected {
				output = &redirected
			}
			for name, confirm := range map[string]func(context.Context) (bool, error){
				"billing": func(ctx context.Context) (bool, error) {
					return NewTerminalApprover(terminal, output).ConfirmFallback(ctx, choice)
				},
				"installation": func(ctx context.Context) (bool, error) {
					return NewTerminalInstaller(terminal, output)(ctx, setup.Request{Tool: "codex"})
				},
			} {
				approved, err := confirm(ctx)
				if approved || !errors.Is(err, test.wantError) {
					t.Fatalf("%s: approved=%v, error=%v; want %v", name, approved, err, test.wantError)
				}
			}
			if redirected.Len() != 0 {
				t.Fatal("confirmation prompt was written to redirected output")
			}
		})
	}
}

func TestTerminalReaderIsBoundedAndCancellationAware(t *testing.T) {
	for _, input := range []string{"yes\n", "yes", "", strings.Repeat("x", 65) + "\n"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.WriteString(input)
		w.Close()
		answer, err := (&terminalConfirmation{file: r}).ReadConfirmation(t.Context())
		r.Close()
		if input == "yes\n" && (answer != "yes" || err != nil) {
			t.Fatal("complete line lost")
		}
		if (input == "yes" || input == "") && !errors.Is(err, io.EOF) {
			t.Fatal("EOF became consent")
		}
		if len(answer) > 64 {
			t.Fatal("unbounded input")
		}
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	// An oversized response without a newline must still honor cancellation
	// while discarding its remaining bytes.
	_, _ = w.WriteString(strings.Repeat("x", 65))
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = (&terminalConfirmation{file: r}).ReadConfirmation(ctx)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) > time.Second {
		t.Fatal("terminal wait ignored deadline")
	}
}

func TestTerminalReaderDiscardsTheWholeOversizedResponse(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	_, err = w.WriteString(strings.Repeat("x", 65) + "yes\nno\n")
	w.Close()
	if err != nil {
		t.Fatal(err)
	}
	reader := &terminalConfirmation{file: r}
	answer, err := reader.ReadConfirmation(t.Context())
	if err != nil || answer != "" {
		t.Fatal("oversized response was not rejected", err)
	}
	answer, err = reader.ReadConfirmation(t.Context())
	if err != nil || answer != "no" {
		t.Fatal("oversized response leaked into the next confirmation", err)
	}
}
