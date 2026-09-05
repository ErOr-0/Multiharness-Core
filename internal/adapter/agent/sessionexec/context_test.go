package sessionexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

// Compaction belongs to the harness. Simulate its relevant consequence here:
// the repair process knows nothing about earlier turns. All required context
// must arrive on stdin, whether the session is resumed or unavailable.
func TestRepairContextSurvivesDiscardedHarnessHistory(t *testing.T) {
	for _, priorSession := range []string{"ses_original", ""} {
		name := "resumed session"
		if priorSession == "" {
			name = "session unavailable"
		}
		t.Run(
			name,
			func(t *testing.T) {
				request := validRepairRequest(t)
				request.Implementation.AgentSessionID = priorSession
				request.Input.SessionID = "unrelated-prior-session"
				request.Repository = &store.RepositoryEvidence{
					Baseline:         store.RepositoryState{Root: request.Input.WorkingDir, Fingerprint: "original-baseline"},
					Current:          store.RepositoryState{Root: request.Input.WorkingDir, Fingerprint: "latest-code"},
					Complete:         true,
					ChangedFiles:     []string{"health.go"},
					PreExistingFiles: []string{"notes.md"},
					Diff:             "latest independently captured diff",
				}
				session := priorSession
				if session == "" {
					session = "ses_rebuilt"
				}
				runner := &fakeProcessRunner{run: func(_ context.Context, command process.Command) (process.Result, error) {
					invocation := captureInvocation(t, command)
					_, payload, found := strings.Cut(invocation.prompt, "Repair request:\n")
					if !found {
						t.Fatal("repair did not carry a context payload")
					}
					var received struct {
						Input            store.TaskInput            `json:"input"`
						Plan             store.Plan                 `json:"plan"`
						Implementation   store.ImplementationResult `json:"implementation"`
						Validation       store.ValidationReport     `json:"validation"`
						Repository       *store.RepositoryEvidence  `json:"repository"`
						ReviewSummary    string                     `json:"review_summary"`
						BlockingFindings []store.ReviewFinding      `json:"blocking_findings"`
					}
					if err := json.NewDecoder(strings.NewReader(payload)).Decode(&received); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(received.Input, request.Input) || !reflect.DeepEqual(received.Plan, request.Plan) ||
						!reflect.DeepEqual(received.Implementation, request.Implementation) || !reflect.DeepEqual(received.Validation, request.Validation) ||
						!reflect.DeepEqual(received.Repository, request.Repository) || received.ReviewSummary != request.Review.Summary ||
						!reflect.DeepEqual(received.BlockingFindings, request.Review.Findings[:1]) {
						t.Fatal("repair cannot reconstruct original intent, latest evidence and blocking feedback from its own prompt")
					}
					if priorSession == "" {
						if slices.Contains(invocation.args, "--session") {
							t.Fatal("fresh repair reused unrelated history")
						}
					} else if argumentValue(t, invocation.args, "--session") != priorSession {
						t.Fatal("repair lost its own session")
					}
					writeOutput(t, command, successfulEventStream(session, "Repaired from supplied context.", "health.go"))
					return process.Result{}, nil
				}}
				implementer, err := NewImplementer(runner, Config{})
				if err != nil {
					t.Fatal(err)
				}
				result, err := implementer.ApplyReview(t.Context(), request)
				if err != nil || result.AgentSessionID != session || runner.calls != 1 {
					t.Fatalf("context handoff failed or replayed: result=%+v calls=%d err=%v", result, runner.calls, err)
				}
			},
		)
	}
}

func TestContextSummaryCannotSubstituteForImplementationResult(t *testing.T) {
	runner := &fakeProcessRunner{run: func(_ context.Context, command process.Command) (process.Result, error) {
		writeOutput(
			t,
			command,
			`{"type":"text","sessionID":"ses_original","part":{"type":"text","text":"Earlier context summarized. Implementation still in progress."}}`+"\n",
		)
		return process.Result{ExitCode: 0}, nil
	}}
	implementer, err := NewImplementer(runner, Config{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := implementer.Implement(t.Context(), validImplementationRequest(t))
	var invalid *OutputError
	if !errors.As(err, &invalid) || result.Summary != "" || runner.calls != 1 {
		t.Fatal("a context summary was accepted as completed implementation or retried")
	}
}
