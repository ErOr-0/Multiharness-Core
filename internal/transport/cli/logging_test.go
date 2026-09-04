package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func TestCorrelatedJSONLogsRedactUnknownMetadata(t *testing.T) {
	var stdout, stderr bytes.Buffer
	const secret = "not-a-recognizable-key-but-still-secret\nforged log"
	h := newHandler(t, func(_ config.Config, sink workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(context.Context, store.TaskInput) store.TaskOutput {
			sink.Publish(workflow.Event{Sequence: 1, Type: workflow.EventTypeStageStarted, Stage: store.WorkflowStageValidation})
			sink.Publish(workflow.Event{Sequence: 2, Type: workflow.EventType(secret), Stage: store.WorkflowStage(secret), Status: store.TaskStatus(secret), FailureCode: store.FailureCode(secret), ProviderKind: store.ProviderFailureKind(secret)})
			// Concurrent publishers must never interleave JSON objects.
			var workers sync.WaitGroup
			for i := range 20 {
				workers.Go(func() {
					sink.Publish(workflow.Event{Sequence: i + 3, Type: workflow.EventTypeStageProgress, Stage: store.WorkflowStageRepair, RepairAttempt: 1, BlockingFindings: 1})
				})
			}
			workers.Wait()
			return exampleOutput(store.TaskStatusAnswered)
		}), nil
	}, &stdout, &stderr, t.TempDir(), map[string]string{"MULTIHARNESS_LOG_FORMAT": "json"})
	var previousTask, previousRun string
	for range 2 {
		stdout.Reset()
		stderr.Reset()
		if code := h.Run(t.Context(), []string{"--task", secret}); code != 0 {
			t.Fatalf("exit=%d", code)
		}
		var result cli.Result
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.SchemaVersion != "1" || !strings.HasPrefix(result.TaskID, "task_") || !strings.HasPrefix(result.RunID, "run_") || result.TaskID == previousTask || result.RunID == previousRun {
			t.Fatal("missing or reused correlation")
		}
		previousTask, previousRun = result.TaskID, result.RunID
		if strings.Contains(stderr.String(), secret) || !strings.Contains(stderr.String(), "[redacted]") {
			t.Fatal("metadata redaction failed")
		}
		decoder := json.NewDecoder(&stderr)
		var count int
		var warning, terminal bool
		for {
			var record struct {
				Version  int
				Time     string
				TaskID   string `json:"task_id"`
				RunID    string `json:"run_id"`
				Code     string
				ExitCode *int `json:"exit_code"`
			}
			if err := decoder.Decode(&record); err == io.EOF {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			count++
			if record.Version != 1 || record.TaskID != result.TaskID || record.RunID != result.RunID {
				t.Fatal("log/result correlation mismatch")
			}
			if _, err := time.Parse(time.RFC3339Nano, record.Time); err != nil {
				t.Fatal(err)
			}
			warning = warning || record.Code == "no_validation_checks"
			terminal = terminal || (record.Code == "result_ready" && record.ExitCode != nil && *record.ExitCode == 0)
		}
		if count != 24 || !warning || !terminal {
			t.Fatalf("records=%d warning=%v terminal=%v", count, warning, terminal)
		}
	}
}

type shortWriter struct{}

func TestRuntimeSelectionNotices(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		var stdout, stderr bytes.Buffer
		h := newHandler(t, func(_ config.Config, sink workflow.EventSink) (cli.Runner, error) {
			return runFunc(func(context.Context, store.TaskInput) store.TaskOutput {
				reporter, ok := sink.(interface{ CodexRuntimeSelected(string) error })
				if !ok {
					t.Fatal("runtime notice reporter missing")
				}
				for _, version := range []string{"0.153.0", "SECRET\nforged"} {
					if err := reporter.CodexRuntimeSelected(version); err != nil {
						t.Fatal(err)
					}
				}
				return exampleOutput(store.TaskStatusAnswered)
			}), nil
		}, &stdout, &stderr, t.TempDir(), nil)
		if code := h.Run(t.Context(), []string{"--log-format", format, "task"}); code != 0 {
			t.Fatal("runtime notice failed")
		}
		if text := stderr.String(); strings.Contains(text, "SECRET") || !strings.Contains(text, "0.153.0") || !strings.Contains(text, "codex_runtime_selected") || !strings.Contains(text, "[redacted]") {
			t.Fatal("missing or unsafe runtime notice")
		}
		if format == "json" {
			for _, line := range bytes.Split(bytes.TrimSpace(stderr.Bytes()), []byte{'\n'}) {
				if !json.Valid(line) {
					t.Fatal("runtime notice broke JSONL logging")
				}
			}
		}
	}
}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }

type secretErrorWriter struct{}

func (secretErrorWriter) Write([]byte) (int, error) { return 0, errors.New("writer-secret") }

func TestLogAndResultWriterFailuresDoNotLeakOrSucceed(t *testing.T) {
	for _, format := range []string{"text", "json"} {
		for _, target := range []string{"stdout", "stderr"} {
			for _, writer := range []io.Writer{shortWriter{}, secretErrorWriter{}} {
				var stdout, stderr bytes.Buffer
				var out, errOut io.Writer = &stdout, &stderr
				if target == "stdout" {
					out = writer
				} else {
					errOut = writer
				}
				h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
					return runFunc(func(context.Context, store.TaskInput) store.TaskOutput {
						return exampleOutput(store.TaskStatusAnswered)
					}), nil
				}, out, errOut, t.TempDir(), nil)
				if code := h.Run(t.Context(), []string{"--log-format", format, "task"}); code != cli.ExitFailed {
					t.Fatalf("writer failure exit=%d", code)
				}
				if strings.Contains(stdout.String()+stderr.String(), "writer-secret") {
					t.Fatal("writer error leaked")
				}
				if target == "stderr" {
					decodeOutput(t, stdout.Bytes())
				}
			}
		}
	}
}

func TestLogFormatValidationAndFlagPrecedence(t *testing.T) {
	var stdout, stderr bytes.Buffer
	h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		return runFunc(func(context.Context, store.TaskInput) store.TaskOutput {
			return exampleOutput(store.TaskStatusAnswered)
		}), nil
	}, &stdout, &stderr, t.TempDir(), map[string]string{"MULTIHARNESS_LOG_FORMAT": "invalid"})
	if code := h.Run(t.Context(), []string{"task"}); code != cli.ExitUsage {
		t.Fatal("accepted unknown log format")
	}
	stdout.Reset()
	stderr.Reset()
	if code := h.Run(t.Context(), []string{"--log-format", "json", "task"}); code != 0 || !json.Valid(bytes.TrimSpace(stderr.Bytes())) {
		t.Fatal("log format precedence failed")
	}
}
