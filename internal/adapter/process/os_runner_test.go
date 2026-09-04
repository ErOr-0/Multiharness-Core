package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	helperEnabled = "MULTIHARNESS_PROCESS_HELPER"
	helperMode    = "MULTIHARNESS_PROCESS_HELPER_MODE"
	helperPIDFile = "MULTIHARNESS_PROCESS_HELPER_PID_FILE"
)

func TestProcessHelper(t *testing.T) {
	if os.Getenv(helperEnabled) != "1" {
		return
	}

	switch os.Getenv(helperMode) {
	case "echo":
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(2)
		}
		_, _ = fmt.Fprintf(os.Stdout, "stdout:%s", input)
		_, _ = fmt.Fprintf(os.Stderr, "stderr:%s", input)
	case "environment":
		_, _ = fmt.Fprintf(
			os.Stdout,
			"%s|%s|%s",
			os.Getenv("MULTIHARNESS_INHERITED"),
			os.Getenv("MULTIHARNESS_OVERRIDDEN"),
			os.Getenv("MULTIHARNESS_ADDED"),
		)
	case "working-directory":
		directory, err := os.Getwd()
		if err != nil {
			os.Exit(2)
		}
		_, _ = fmt.Fprintf(os.Stdout, "%s|%s", directory, os.Getenv("PWD"))
	case "exit":
		_, _ = fmt.Fprint(os.Stdout, "stdout-before-exit")
		_, _ = fmt.Fprint(os.Stderr, "stderr-before-exit")
		os.Exit(7)
	case "ready-sleep":
		_, _ = fmt.Fprintln(os.Stdout, "ready")
		_ = os.Stdout.Sync()
		time.Sleep(30 * time.Second)
	case "stream":
		_, _ = fmt.Fprintln(os.Stdout, "first")
		_ = os.Stdout.Sync()
		time.Sleep(250 * time.Millisecond)
		_, _ = fmt.Fprintln(os.Stdout, "second")
	case "large-output":
		_, _ = fmt.Fprint(os.Stdout, "stdout-head-"+strings.Repeat("x", 128)+"-stdout-tail")
		_, _ = fmt.Fprint(os.Stderr, "stderr-head-"+strings.Repeat("y", 128)+"-stderr-tail")
	case "tree-parent":
		runTreeParentHelper()
	case "tree-child":
		time.Sleep(30 * time.Second)
	default:
		_, _ = fmt.Fprintf(os.Stderr, "unknown helper mode %q", os.Getenv(helperMode))
		os.Exit(2)
	}
	os.Exit(0)
}

func TestOSRunnerUsesStdinAndSeparateStreams(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := helperCommand(t, "echo")
	command.Stdin = strings.NewReader("payload")
	command.Stdout = &stdout
	command.Stderr = &stderr

	result, err := NewOSRunner().Run(context.Background(), command)
	if err != nil {
		t.Fatalf("Run() returned an error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d; want 0", result.ExitCode)
	}
	if result.Stdout != "stdout:payload" || stdout.String() != result.Stdout {
		t.Fatalf("stdout capture = %q, stream = %q", result.Stdout, stdout.String())
	}
	if result.Stderr != "stderr:payload" || stderr.String() != result.Stderr {
		t.Fatalf("stderr capture = %q, stream = %q", result.Stderr, stderr.String())
	}
}

func TestOSRunnerSerializesSharedOutputSink(t *testing.T) {
	var combined bytes.Buffer
	command := helperCommand(t, "echo")
	command.Stdin = strings.NewReader("payload")
	command.Stdout = &combined
	command.Stderr = &combined

	if _, err := NewOSRunner().Run(context.Background(), command); err != nil {
		t.Fatalf("Run() returned an error: %v", err)
	}
	if output := combined.String(); !strings.Contains(output, "stdout:payload") || !strings.Contains(output, "stderr:payload") {
		t.Fatalf("shared output = %q; expected both streams", output)
	}
}

func TestOSRunnerInheritsAndOverridesEnvironment(t *testing.T) {
	t.Setenv("MULTIHARNESS_INHERITED", "inherited")
	t.Setenv("MULTIHARNESS_OVERRIDDEN", "old")
	command := helperCommand(t, "environment")
	command.EnvOverrides["MULTIHARNESS_OVERRIDDEN"] = "new"
	command.EnvOverrides["MULTIHARNESS_ADDED"] = "added"

	result, err := NewOSRunner().Run(context.Background(), command)
	if err != nil {
		t.Fatalf("Run() returned an error: %v", err)
	}
	if result.Stdout != "inherited|new|added" {
		t.Fatalf("Stdout = %q; want inherited environment plus overrides", result.Stdout)
	}
}

func TestOSRunnerUsesWorkingDirectory(t *testing.T) {
	directory := t.TempDir()
	command := helperCommand(t, "working-directory")
	command.Dir = directory

	result, err := NewOSRunner().Run(context.Background(), command)
	if err != nil {
		t.Fatalf("Run() returned an error: %v", err)
	}
	resolvedDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks() returned an error: %v", err)
	}
	parts := strings.Split(result.Stdout, "|")
	if len(parts) != 2 {
		t.Fatalf("Stdout = %q; expected working directory %q", result.Stdout, resolvedDirectory)
	}
	resolvedCWD, err := filepath.EvalSymlinks(parts[0])
	if err != nil || resolvedCWD != resolvedDirectory {
		t.Fatalf("working directory = %q; want %q", parts[0], resolvedDirectory)
	}
	if runtime.GOOS != "windows" {
		resolvedPWD, resolveErr := filepath.EvalSymlinks(parts[1])
		if resolveErr != nil || resolvedPWD != resolvedDirectory {
			t.Fatalf("PWD = %q; want %q", parts[1], resolvedDirectory)
		}
	}
}

func TestOSRunnerUnsetsInheritedEnvironmentBeforeOverrides(t *testing.T) {
	t.Setenv("MULTIHARNESS_INHERITED", "remove-me")
	t.Setenv("MULTIHARNESS_OVERRIDDEN", "old")
	command := helperCommand(t, "environment")
	command.EnvUnset = []string{"MULTIHARNESS_INHERITED", "MULTIHARNESS_OVERRIDDEN"}
	command.EnvOverrides["MULTIHARNESS_OVERRIDDEN"] = "new"
	result, err := NewOSRunner().Run(t.Context(), command)
	if err != nil || result.Stdout != "|new|" {
		t.Fatalf("unset/override: %#v %v", result, err)
	}
}

func TestOSRunnerStreamsBeforeCompletion(t *testing.T) {
	stream := make(chan string, 4)
	command := helperCommand(t, "stream")
	command.Stdout = channelWriter{channel: stream}

	type outcome struct {
		result Result
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := NewOSRunner().Run(context.Background(), command)
		done <- outcome{result: result, err: err}
	}()

	select {
	case chunk := <-stream:
		if !strings.Contains(chunk, "first") {
			t.Fatalf("first streamed chunk = %q; expected first output", chunk)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for streamed output")
	}

	select {
	case <-done:
		t.Fatal("Run() completed before the delayed second output")
	default:
	}

	select {
	case outcome := <-done:
		if outcome.err != nil {
			t.Fatalf("Run() returned an error: %v", outcome.err)
		}
		if outcome.result.Stdout != "first\nsecond\n" {
			t.Fatalf("Stdout = %q; want both streamed messages", outcome.result.Stdout)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for command completion")
	}
}

func TestOSRunnerBoundsRetainedOutput(t *testing.T) {
	const limit = 24
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command := helperCommand(t, "large-output")
	command.OutputLimit = limit
	command.Stdout = &stdout
	command.Stderr = &stderr

	result, err := NewOSRunner().Run(context.Background(), command)
	if err != nil {
		t.Fatalf("Run() returned an error: %v", err)
	}
	if len(result.Stdout) != limit || !strings.HasSuffix(result.Stdout, "-stdout-tail") {
		t.Fatalf("bounded Stdout = %q; want %d trailing bytes", result.Stdout, limit)
	}
	if len(result.Stderr) != limit || !strings.HasSuffix(result.Stderr, "-stderr-tail") {
		t.Fatalf("bounded Stderr = %q; want %d trailing bytes", result.Stderr, limit)
	}
	if !result.StdoutTruncated || !result.StderrTruncated {
		t.Fatal("truncation flags = false; want true for both streams")
	}
	if stdout.Len() <= limit || stderr.Len() <= limit {
		t.Fatal("streamed output was unexpectedly limited")
	}
}

func TestOSRunnerClassifiesCommandFailures(t *testing.T) {
	missingDirectory := filepath.Join(t.TempDir(), "missing")
	regularFile := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(regularFile, []byte("test"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() returned an error: %v", err)
	}

	nonZeroCommand := helperCommand(t, "exit")
	tests := []struct {
		name         string
		command      Command
		wantKind     ErrorKind
		wantExitCode int
	}{
		{name: "blank command", command: Command{}, wantKind: ErrorKindInvalidCommand, wantExitCode: -1},
		{
			name:     "missing executable",
			command:  Command{Name: filepath.Join(t.TempDir(), "missing-executable")},
			wantKind: ErrorKindExecutableNotFound, wantExitCode: -1,
		},
		{
			name:     "missing working directory",
			command:  Command{Name: nonZeroCommand.Name, Dir: missingDirectory},
			wantKind: ErrorKindWorkingDirectory, wantExitCode: -1,
		},
		{
			name:     "working directory is a file",
			command:  Command{Name: nonZeroCommand.Name, Dir: regularFile},
			wantKind: ErrorKindWorkingDirectory, wantExitCode: -1,
		},
		{name: "non-zero exit", command: nonZeroCommand, wantKind: ErrorKindNonZeroExit, wantExitCode: 7},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := NewOSRunner().Run(context.Background(), test.command)
			runErr := assertRunErrorKind(t, err, test.wantKind)
			if result.ExitCode != test.wantExitCode || runErr.ExitCode != test.wantExitCode {
				t.Fatalf("exit codes = result %d, error %d; want %d", result.ExitCode, runErr.ExitCode, test.wantExitCode)
			}
			if test.wantKind == ErrorKindNonZeroExit {
				if result.Stdout != "stdout-before-exit" || result.Stderr != "stderr-before-exit" {
					t.Fatalf("failure output = stdout %q, stderr %q", result.Stdout, result.Stderr)
				}
			}
		})
	}
}

func TestOSRunnerRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		command Command
	}{
		{name: "nil context", command: helperCommand(t, "echo")},
		{
			name:    "negative timeout",
			ctx:     context.Background(),
			command: Command{Name: "command", Timeout: -time.Second},
		},
		{
			name:    "negative output limit",
			ctx:     context.Background(),
			command: Command{Name: "command", OutputLimit: -1},
		},
		{
			name:    "invalid environment key",
			ctx:     context.Background(),
			command: Command{Name: "command", EnvOverrides: map[string]string{"BAD=KEY": "value"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewOSRunner().Run(test.ctx, test.command)
			assertRunErrorKind(t, err, ErrorKindInvalidCommand)
		})
	}
}

func TestOSRunnerDistinguishesTimeoutAndCancellation(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		command := helperCommand(t, "ready-sleep")
		command.Timeout = 100 * time.Millisecond

		_, err := NewOSRunner().Run(context.Background(), command)
		assertRunErrorKind(t, err, ErrorKindTimeout)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v; expected context deadline", err)
		}
	})

	t.Run("cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		stream := make(chan string, 2)
		command := helperCommand(t, "ready-sleep")
		command.Stdout = channelWriter{channel: stream}
		done := make(chan error, 1)
		go func() {
			_, err := NewOSRunner().Run(ctx, command)
			done <- err
		}()

		select {
		case <-stream:
			cancel()
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("timed out waiting for helper readiness")
		}

		select {
		case err := <-done:
			assertRunErrorKind(t, err, ErrorKindCancelled)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("error = %v; expected context cancellation", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for cancellation")
		}
	})
}

func TestOSRunnerReportsStreamingSinkFailure(t *testing.T) {
	sinkError := errors.New("sink failed")
	command := helperCommand(t, "echo")
	command.Stdin = strings.NewReader("payload")
	command.Stdout = failingWriter{err: sinkError}

	result, err := NewOSRunner().Run(context.Background(), command)
	assertRunErrorKind(t, err, ErrorKindOutput)
	if !errors.Is(err, sinkError) {
		t.Fatalf("error = %v; expected sink error", err)
	}
	if result.Stdout != "stdout:payload" {
		t.Fatalf("Stdout = %q; expected captured output despite sink failure", result.Stdout)
	}
}

func helperCommand(t *testing.T, mode string) Command {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() returned an error: %v", err)
	}
	return Command{
		Name: executable,
		Args: []string{"-test.run=^TestProcessHelper$"},
		EnvOverrides: map[string]string{
			helperEnabled: "1",
			helperMode:    mode,
		},
		Timeout: 5 * time.Second,
	}
}

func runTreeParentHelper() {
	executable, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	child := exec.Command(executable, "-test.run=^TestProcessHelper$")
	child.Env, err = mergeEnvironment(os.Environ(), map[string]string{
		helperEnabled: "1",
		helperMode:    "tree-child",
	})
	if err != nil || child.Start() != nil {
		os.Exit(2)
	}
	pidFile := os.Getenv(helperPIDFile)
	if pidFile == "" || os.WriteFile(pidFile, []byte(strconv.Itoa(child.Process.Pid)), 0o600) != nil {
		_ = child.Process.Kill()
		os.Exit(2)
	}
	if child.Wait() != nil {
		os.Exit(3)
	}
}

func assertRunErrorKind(t *testing.T, err error, expected ErrorKind) *RunError {
	t.Helper()
	if err == nil {
		t.Fatalf("Run() returned nil; want %s", expected)
	}
	var runErr *RunError
	if !errors.As(err, &runErr) {
		t.Fatalf("error type = %T; want *RunError", err)
	}
	if runErr.Kind != expected {
		t.Fatalf("error kind = %q; want %q (error: %v)", runErr.Kind, expected, err)
	}
	return runErr
}

type channelWriter struct {
	channel chan<- string
}

func (writer channelWriter) Write(data []byte) (int, error) {
	writer.channel <- string(append([]byte(nil), data...))
	return len(data), nil
}

type failingWriter struct {
	err error
}

func (writer failingWriter) Write([]byte) (int, error) {
	return 0, writer.err
}
