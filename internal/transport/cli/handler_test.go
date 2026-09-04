package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

type runFunc func(context.Context, store.TaskInput) store.TaskOutput

func (f runFunc) Run(ctx context.Context, input store.TaskInput) store.TaskOutput {
	return f(ctx, input)
}

func newHandler(t *testing.T, factory cli.Factory, stdout, stderr io.Writer, base string, env map[string]string) *cli.Handler {
	t.Helper()
	h, err := cli.NewHandler(factory, stdout, stderr, base, func(key string) (string, bool) { v, ok := env[key]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func decodeOutput(t *testing.T, data []byte) store.TaskOutput {
	t.Helper()
	var output store.TaskOutput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&output); err != nil {
		t.Fatalf("bad JSON output %q: %v", data, err)
	}
	if err := output.Validate(); err != nil {
		t.Fatalf("invalid output: %v; %s", err, data)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stdout must contain exactly one JSON document: %s", data)
	}
	return output
}

func exampleOutput(status store.TaskStatus) store.TaskOutput {
	output := store.TaskOutput{Status: status, Summary: "result"}
	switch status {
	case store.TaskStatusFailed:
		output.Failure = &store.TaskFailure{Stage: store.WorkflowStagePlanning, Code: store.FailureCodeAgent, Message: "agent failed"}
	case store.TaskStatusAnswered:
		state := store.RepositoryState{Root: "/repo", Fingerprint: "same"}
		output.Repository = &store.RepositoryEvidence{Baseline: state, Current: state, Complete: true}
		output.Plan = &store.Plan{Action: store.PlanActionAnswer, Summary: "summary", Answer: output.Summary}
	case store.TaskStatusApproved, store.TaskStatusRepairLimitReached:
		state := store.RepositoryState{Root: "/repo", Fingerprint: "same"}
		output.Repository = &store.RepositoryEvidence{Baseline: state, Current: state, Complete: true}
		output.Plan = &store.Plan{Action: store.PlanActionImplement, Summary: "summary", Steps: []string{"step"}, AcceptanceCriteria: []string{"pass"}}
		output.Implementation = &store.ImplementationResult{Summary: "implemented"}
		output.Validation = &store.ValidationReport{Passed: true}
		output.LastReview = &store.Review{Approved: status == store.TaskStatusApproved, Summary: "review"}
		if !output.LastReview.Approved {
			output.LastReview.Findings = []store.ReviewFinding{{Severity: store.FindingSeverityError, Blocking: true, Description: "broken", Evidence: "failed check", RequiredAction: "fix"}}
		}
	}
	return output
}

func TestCLIMapsStatusesAndKeepsProgressOffStdout(t *testing.T) {
	for status, want := range map[store.TaskStatus]int{store.TaskStatusApproved: 0, store.TaskStatusAnswered: 0, store.TaskStatusFailed: 1, store.TaskStatusCancelled: 130, store.TaskStatusRepairLimitReached: 3} {
		t.Run(string(status), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			factory := func(cfg config.Config, sink workflow.EventSink) (cli.Runner, error) {
				return runFunc(func(ctx context.Context, input store.TaskInput) store.TaskOutput {
					if input.Task != "sensitive task text" || input.WorkingDir != cfg.WorkingDir {
						t.Fatal("lost input")
					}
					sink.Publish(workflow.Event{Sequence: 1, Stage: store.WorkflowStagePlanning, Type: workflow.EventTypeStageStarted})
					return exampleOutput(status)
				}), nil
			}
			h := newHandler(t, factory, &stdout, &stderr, t.TempDir(), nil)
			if code := h.Run(t.Context(), []string{"--task", "sensitive task text"}); code != want {
				t.Fatalf("exit=%d, want=%d; %s", code, want, stdout.String())
			}
			if output := decodeOutput(t, stdout.Bytes()); output.Status != status {
				t.Fatalf("status=%s", output.Status)
			}
			if !strings.Contains(stderr.String(), "planning stage_started") || strings.Contains(stderr.String(), "sensitive") {
				t.Fatalf("stderr: %s", stderr.String())
			}
		})
	}
}

func TestCLIRejectsBadInputBeforeCreatingAgents(t *testing.T) {
	for _, args := range [][]string{
		{}, {"--task", ""}, {"--task", "one", "two"}, {"one", "two"}, {"--task", "x", "--task-file", "x"},
		{"--unknown", "value"}, {"--config", "", "task"}, {"--task-file", "-"}, {"--task-file", "missing"},
		{"--max-task-bytes", "2", "--task", "large"}, {"--task", "\xff"}, {"--task", "\x00"},
		{"--max-repair-attempts", "-1", "task"}, {"--planner-model", "", "task"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
				t.Fatal("created agents for invalid input")
				return nil, nil
			}, &stdout, &stderr, t.TempDir(), nil)
			if code := h.Run(t.Context(), args); code != cli.ExitUsage {
				t.Fatalf("exit=%d: %s", code, stdout.String())
			}
			if output := decodeOutput(t, stdout.Bytes()); output.Status != store.TaskStatusFailed {
				t.Fatal("bad input was not failed")
			}
		})
	}
}

func TestCLILoadsFileEnvironmentFlagsAndTaskFile(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "settings.json"), []byte(`{"version":1,"planner":{"model":"file-model"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "task.txt"), []byte("Explain this repository.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	h := newHandler(t, func(cfg config.Config, sink workflow.EventSink) (cli.Runner, error) {
		if cfg.Planner.Model != "flag-model" || cfg.Reviewer.Model != "env-reviewer" || cfg.MaxRepairAttempts != 0 {
			t.Fatalf("config: %#v", cfg)
		}
		return runFunc(func(_ context.Context, input store.TaskInput) store.TaskOutput {
			if input.Task != "Explain this repository.\n" {
				t.Fatal("task file altered")
			}
			sink.Publish(workflow.Event{Sequence: 1, Stage: store.WorkflowStagePlanning, Type: workflow.EventTypeStageStarted})
			return exampleOutput(store.TaskStatusAnswered)
		}), nil
	}, &stdout, &stderr, base, map[string]string{"MULTIHARNESS_CONFIG": "settings.json", "MULTIHARNESS_PLANNER_MODEL": "env-model", "MULTIHARNESS_REVIEWER_MODEL": "env-reviewer"})
	code := h.Run(t.Context(), []string{"--quiet", "--planner-model", "flag-model", "--max-repair-attempts", "0", "--task-file", "task.txt"})
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	decodeOutput(t, stdout.Bytes())
}

func TestHelpDoesNotLoadConfigOrAgents(t *testing.T) {
	var stdout, stderr bytes.Buffer
	h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
		t.Fatal("help initialized agents")
		return nil, nil
	}, &stdout, &stderr, t.TempDir(), map[string]string{"MULTIHARNESS_CONFIG": "missing"})
	if code := h.Run(t.Context(), []string{"--help"}); code != 0 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(stdout.String(), "Usage:") || !strings.Contains(stdout.String(), "MULTIHARNESS_PLANNER_MODEL") || stderr.Len() != 0 {
		t.Fatalf("help: %s", stdout.String())
	}
}

func TestCLIHoldsWholeRunDeadlineAndHonorsPreCancellation(t *testing.T) {
	for _, preCancelled := range []bool{false, true} {
		t.Run(map[bool]string{true: "before startup", false: "during run"}[preCancelled], func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			h := newHandler(t, func(config.Config, workflow.EventSink) (cli.Runner, error) {
				if preCancelled {
					t.Fatal("cancelled run initialized agents")
				}
				return runFunc(func(ctx context.Context, _ store.TaskInput) store.TaskOutput {
					<-ctx.Done()
					return exampleOutput(store.TaskStatusCancelled)
				}), nil
			}, &stdout, &stderr, t.TempDir(), nil)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if preCancelled {
				cancel()
			}
			if code := h.Run(ctx, []string{"--timeout", "1ms", "task"}); code != 130 {
				t.Fatalf("exit=%d", code)
			}
			decodeOutput(t, stdout.Bytes())
		})
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestCLIDoesNotReportSuccessOnBrokenOutputOrInvalidRunner(t *testing.T) {
	for _, scenario := range []string{"stdout", "stderr", "nil runner", "factory error", "invalid result"} {
		t.Run(scenario, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			var out, errOut io.Writer = &stdout, &stderr
			if scenario == "stdout" {
				out = brokenWriter{}
			}
			if scenario == "stderr" {
				errOut = brokenWriter{}
			}
			h := newHandler(t, func(_ config.Config, sink workflow.EventSink) (cli.Runner, error) {
				if scenario == "nil runner" {
					return nil, nil
				}
				if scenario == "factory error" {
					return nil, errors.New("bad factory")
				}
				return runFunc(func(ctx context.Context, _ store.TaskInput) store.TaskOutput {
					sink.Publish(workflow.Event{Sequence: 1, Stage: store.WorkflowStagePlanning, Type: workflow.EventTypeStageStarted})
					if scenario == "stderr" && ctx.Err() == nil {
						t.Fatal("broken progress must cancel execution")
					}
					if scenario == "invalid result" {
						return store.TaskOutput{Status: store.TaskStatusApproved}
					}
					return exampleOutput(store.TaskStatusAnswered)
				}), nil
			}, out, errOut, t.TempDir(), nil)
			if code := h.Run(t.Context(), []string{"task"}); code == 0 {
				t.Fatal("reported success")
			}
			if scenario != "stdout" {
				output := decodeOutput(t, stdout.Bytes())
				if output.Status != store.TaskStatusFailed {
					t.Fatal("reported non-failure")
				}
			}
		})
	}
}
