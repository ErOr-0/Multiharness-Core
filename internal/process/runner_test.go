package process

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestOSRunnerRunsCommand(t *testing.T) {
	runner := OSRunner{}

	result, err := runner.Run(context.Background(), Command{
		Name:    "go",
		Args:    []string{"version"},
		Timeout: 10 * time.Second,
	})

	if err != nil {
		t.Fatalf("Run() returned an error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d; want 0", result.ExitCode)
	}

	if !strings.Contains(result.Output, "go version") {
		t.Fatalf("Output = %q; expected Go version", result.Output)
	}
}

func TestOSRunnerReturnsNonZeroExitCode(t *testing.T) {
	runner := OSRunner{}

	result, err := runner.Run(context.Background(), Command{
		Name:    "go",
		Args:    []string{"definitely-not-a-command"},
		Timeout: 10 * time.Second,
	})

	if err == nil {
		t.Fatal("Run() returned nil error; expected command failure")
	}

	if result.ExitCode == 0 {
		t.Fatal("ExitCode = 0; expected non-zero exit code")
	}

	if result.Output == "" {
		t.Fatal("Output is empty; expected command error output")
	}
}
