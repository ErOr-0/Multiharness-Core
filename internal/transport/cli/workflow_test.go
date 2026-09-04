package cli_test

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

// These are agent/workspace fakes, not a fake orchestrator: CLI tests execute
// the same workflow.Service used by the production composition root.
type fixture struct {
	base             store.RepositoryState
	current          store.RepositoryEvidence
	calls            []string
	repaired, closed bool
}

func (f *fixture) Validate(context.Context, string) error                             { return nil }
func (f *fixture) Acquire(context.Context, string) (workflow.WorkspaceSession, error) { return f, nil }
func (f *fixture) Baseline() store.RepositoryEvidence {
	return store.RepositoryEvidence{Baseline: f.base, Current: f.base, Complete: true}
}
func (f *fixture) Inspect(context.Context) (store.RepositoryEvidence, error) {
	return *f.current.Clone(), nil
}
func (f *fixture) Close() error { f.closed = true; return nil }
func (f *fixture) Plan(_ context.Context, input store.TaskInput) (store.Plan, error) {
	f.calls = append(f.calls, "plan")
	if input.Task == "explain" {
		return store.Plan{Action: store.PlanActionAnswer, Summary: "explanation", Answer: "Here is the explanation."}, nil
	}
	return store.Plan{Action: store.PlanActionImplement, Summary: "change", Steps: []string{"implement"}, AcceptanceCriteria: []string{"passes"}}, nil
}
func (f *fixture) Implement(context.Context, store.ImplementationRequest) (store.ImplementationResult, error) {
	f.calls = append(f.calls, "implement")
	f.current.Current.Fingerprint = "implemented"
	f.current.ChangedFiles = []string{"file.go"}
	return store.ImplementationResult{Summary: "implemented", AgentSessionID: "session"}, nil
}
func (f *fixture) ApplyReview(_ context.Context, request store.RepairRequest) (store.ImplementationResult, error) {
	f.calls = append(f.calls, "repair")
	f.repaired = true
	if request.Implementation.AgentSessionID != "session" || request.Validation.Passed || request.Review.Approved {
		panic("repair context lost")
	}
	f.current.Current.Fingerprint = "repaired"
	return store.ImplementationResult{Summary: "repaired", AgentSessionID: "session"}, nil
}

type fixtureValidator struct{ fixture *fixture }

func (v fixtureValidator) Validate(context.Context, store.ValidationRequest) (store.ValidationReport, error) {
	f := v.fixture
	f.calls = append(f.calls, "validate")
	exit := 1
	if f.repaired {
		exit = 0
	}
	return store.ValidationReport{Passed: f.repaired, Checks: []store.ValidationEvidence{{Command: "test", Passed: f.repaired, ExitCode: exit}}}, nil
}
func (f *fixture) Review(context.Context, store.ReviewRequest) (store.Review, error) {
	f.calls = append(f.calls, "review")
	if f.repaired {
		return store.Review{Approved: true, Summary: "verified"}, nil
	}
	return store.Review{Summary: "fix required", Findings: []store.ReviewFinding{{Severity: store.FindingSeverityError, Blocking: true, Description: "broken", Evidence: "test failed", RequiredAction: "fix"}}}, nil
}

func TestCLIExecutesRepairLimitAndAnswerBranchesThroughService(t *testing.T) {
	for _, test := range []struct {
		name, task, limit string
		status            store.TaskStatus
		exit              int
		calls             []string
	}{
		{"repair", "change", "1", store.TaskStatusApproved, 0, []string{"plan", "implement", "validate", "review", "repair", "validate", "review"}},
		{"limit", "change", "0", store.TaskStatusRepairLimitReached, 3, []string{"plan", "implement", "validate", "review"}},
		{"answer", "explain", "3", store.TaskStatusAnswered, 0, []string{"plan"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			f := &fixture{}
			h := newHandler(t, func(cfg config.Config, sink workflow.EventSink) (cli.Runner, error) {
				f.base = store.RepositoryState{Root: cfg.WorkingDir, Fingerprint: "baseline"}
				f.current = f.Baseline()
				return workflow.NewService(workflow.Dependencies{Workspace: f, Planner: f, Implementer: f, Validator: fixtureValidator{f}, Reviewer: f, Events: sink})
			}, &stdout, &stderr, t.TempDir(), nil)
			if exit := h.Run(t.Context(), []string{"--max-repair-attempts", test.limit, test.task}); exit != test.exit {
				t.Fatalf("exit=%d: %s", exit, stdout.String())
			}
			if output := decodeOutput(t, stdout.Bytes()); output.Status != test.status {
				t.Fatalf("status=%s", output.Status)
			}
			if !reflect.DeepEqual(f.calls, test.calls) {
				t.Fatalf("calls=%v", f.calls)
			}
			if !f.closed {
				t.Fatal("workspace not released")
			}
		})
	}
}
