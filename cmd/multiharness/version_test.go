package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"multiharness-core/internal/transport/cli"
)

func TestVersionWorksWithoutConfiguredAgentsOrRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("MULTIHARNESS_PLANNER_HARNESS", "invalid-provider")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != cli.ExitSuccess {
		t.Fatalf("version exited %d: %s", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "magent ") || !strings.Contains(stdout.String(), version) || stderr.Len() != 0 {
		t.Fatalf("missing version or unexpected diagnostics: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestVersionOutputFailureReturnsFailure(t *testing.T) {
	reader, writer := io.Pipe()
	_ = reader.Close()
	defer writer.Close()
	if code := run([]string{"--version"}, writer, io.Discard); code != cli.ExitFailed {
		t.Fatalf("version exited %d after output failure", code)
	}
}
