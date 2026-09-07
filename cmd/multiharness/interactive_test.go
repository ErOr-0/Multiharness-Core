package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/transport/cli"
)

type interactiveFixtureInput struct{ lines []string }

func (p *interactiveFixtureInput) ReadLine(context.Context, int) (string, error) {
	if len(p.lines) == 0 {
		return "", io.EOF
	}
	line := p.lines[0]
	p.lines = p.lines[1:]
	return line, nil
}

func TestInteractiveWorkflowIntegration(t *testing.T) {
	cfg, log := fixtureConfiguration(t)
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(settings, data, 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	h, err := cli.NewHandler(buildWorkflow, &stdout, &stderr, cfg.WorkingDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := &interactiveFixtureInput{lines: []string{"Fix result.txt and verify it", "/quit"}}
	if code := h.Interactive(t.Context(), input, settings); code != 0 {
		t.Fatalf("exit=%d\n%s\n%s", code, stdout.String(), stderr.String())
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(calls) != "plan\nimplement\ncheck\nreview\nrepair\ncheck\nreview\n" || !strings.Contains(stdout.String(), "\napproved\n") {
		t.Fatalf("incomplete repair workflow: %s\n%s\n%s", calls, stdout.String(), stderr.String())
	}
	notes, err := os.ReadFile(filepath.Join(cfg.WorkingDir, "notes.txt"))
	if err != nil || string(notes) != "user notes\n" {
		t.Fatal("interactive workflow lost pre-existing notes")
	}
}
