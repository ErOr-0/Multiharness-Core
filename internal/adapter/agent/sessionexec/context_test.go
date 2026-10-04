package sessionexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/contract"
)

func TestPlannerFindingsReachFreshImplementationProcess(t *testing.T) {
	plan, err := structured.ParsePlan([]byte(`{"schema_version":"3","action":"implement","answer":"","summary":"Update the health endpoint","handoff_context":["internal/http/health.go owns the route","Keep the existing readiness response unchanged"],"steps":["Add the requested status field"],"acceptance_criteria":["The health test passes"]}`))
	if err != nil {
		t.Fatal(err)
	}
	request := validImplementationRequest(t)
	request.Input.Task = "Add the status field without changing readiness"
	request.Input.RecentTurns = []contract.ConversationTurn{{User: "Which endpoint should change?", Assistant: "The health endpoint."}}
	request.Plan = plan
	runner := &fakeProcessRunner{run: func(_ context.Context, command process.Command) (process.Result, error) {
		invocation := captureInvocation(t, command)
		if slices.Contains(invocation.args, "--session") {
			t.Fatal("initial implementation unexpectedly reused planner history")
		}
		_, payload, found := strings.Cut(invocation.prompt, "Implementation request:\n")
		if !found {
			t.Fatal("implementation request is missing")
		}
		// The bounded projection carries task, recent turns, plan and workspace
		// identity under the "handoff" key the instructions name; canonical
		// diffs stay in workflow state.
		var received struct {
			Handoff contract.ImplementationHandoff `json:"handoff"`
		}
		if err := json.NewDecoder(strings.NewReader(payload)).Decode(&received); err != nil {
			t.Fatal(err)
		}
		handoff := received.Handoff
		if handoff.Task != request.Input.Task || !reflect.DeepEqual(handoff.RecentTurns, request.Input.RecentTurns) || !reflect.DeepEqual(handoff.Plan.HandoffContext, plan.HandoffContext) {
			t.Fatalf("planner findings or original request were lost: %#v", handoff)
		}
		if strings.Contains(invocation.prompt, `"pre_existing_files"`) || strings.Contains(invocation.prompt, `"diff":`) {
			t.Fatal("implementation prompt carries unbounded evidence")
		}
		writeOutput(t, command, successfulEventStream("ses_new", "Updated from handoff.", "health.go"))
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := implementer.Implement(t.Context(), request); err != nil {
		t.Fatal(err)
	}
}

func TestPreviousQuestionReachesFreshAnsweringProcess(t *testing.T) {
	input := validImplementationRequest(t).Input
	input.AnswerOnly = true
	input.Task = "What question did I ask before?"
	input.RecentTurns = []contract.ConversationTurn{{User: "Is there a new GPT model?", Assistant: "The model lineup includes GPT-6."}}
	runner := &fakeProcessRunner{run: func(_ context.Context, command process.Command) (process.Result, error) {
		invocation := captureInvocation(t, command)
		_, payload, found := strings.Cut(invocation.prompt, "Question request:\n")
		if !found {
			t.Fatal("answering request is missing")
		}
		var received contract.TaskInput
		if err := json.NewDecoder(strings.NewReader(payload)).Decode(&received); err != nil {
			t.Fatal(err)
		}
		if received.Task != input.Task || !reflect.DeepEqual(received.RecentTurns, input.RecentTurns) {
			t.Fatalf("answering model lost the previous question: %+v", received)
		}
		response := `{"schema_version":"3","action":"answer","answer":"You asked whether there is a new GPT model.","summary":"recalled question","handoff_context":[],"steps":[],"acceptance_criteria":[]}`
		line, _ := json.Marshal(map[string]any{"type": "text", "sessionID": "new-session", "part": map[string]string{"type": "text", "text": response}})
		writeOutput(t, command, string(line)+"\n"+`{"type":"step_finish","sessionID":"new-session","part":{"type":"step-finish","reason":"stop"}}`+"\n")
		return process.Result{}, nil
	}}
	agent, err := NewReadOnlyAgent(runner, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := agent.Plan(t.Context(), input)
	if err != nil || plan.Action != contract.PlanActionAnswer || !strings.Contains(plan.Answer, "new GPT model") {
		t.Fatal(plan, err)
	}
}

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
				request.Repository = &contract.RepositoryEvidence{
					Baseline:         contract.RepositoryState{Root: request.Input.WorkingDir, Fingerprint: "original-baseline"},
					Current:          contract.RepositoryState{Root: request.Input.WorkingDir, Fingerprint: "latest-code"},
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
					// The delta-oriented projection carries intent, manifest,
					// blocking findings and relevant hunks; the repair agent
					// reads live workspace files itself.
					var received struct {
						Handoff contract.RepairHandoff `json:"handoff"`
					}
					if err := json.NewDecoder(strings.NewReader(payload)).Decode(&received); err != nil {
						t.Fatal(err)
					}
					handoff := received.Handoff
					if handoff.Task != request.Input.Task || !reflect.DeepEqual(handoff.RecentTurns, request.Input.RecentTurns) ||
						!reflect.DeepEqual(handoff.Plan, request.Plan) ||
						handoff.ImplementationSummary != request.Implementation.Summary ||
						!reflect.DeepEqual(handoff.ChangedFiles, request.Implementation.ChangedFiles) ||
						!reflect.DeepEqual(handoff.BlockingFindings, request.Review.Findings[:1]) ||
						handoff.WorkspaceRoot != request.Input.WorkingDir ||
						handoff.WorkspaceFingerprint != "latest-code" ||
						!strings.Contains(handoff.RelevantDiff, "latest independently captured diff") ||
						len(handoff.FailedValidation) != 1 ||
						handoff.FailedValidation[0].Output != "health_test.go:42: expected 200" {
						t.Fatal("repair cannot reconstruct original intent, latest evidence and blocking feedback from its own prompt")
					}
					if strings.Contains(invocation.prompt, `"pre_existing_files"`) {
						t.Fatal("repair prompt carries the complete pre-existing file list")
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

// Repair rounds accumulate in the resumed OpenCode session. When the provider
// rejects that context, the self-contained repair handoff continues fresh.
func TestRepairContextOverflowContinuesInFreshSession(t *testing.T) {
	runner := &fakeProcessRunner{run: func(_ context.Context, command process.Command) (process.Result, error) {
		invocation := captureInvocation(t, command)
		if slices.Contains(invocation.args, "--session") {
			writeOutput(t, command, `{"type":"error","sessionID":"ses_original","error":{"name":"APIError","data":{"message":"prompt is too long: 210000 tokens > 200000 maximum"}}}`+"\n")
			return process.Result{}, nil
		}
		writeOutput(t, command, successfulEventStream("ses_fresh", "Repaired in a fresh session.", "health.go"))
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := implementer.ApplyReview(t.Context(), validRepairRequest(t))
	if err != nil || result.AgentSessionID != "ses_fresh" || runner.calls != 2 {
		t.Fatalf("result=%+v calls=%d err=%v", result, runner.calls, err)
	}
}

func TestOpenCodeErrorEventIsClassified(t *testing.T) {
	runner := &fakeProcessRunner{run: func(_ context.Context, command process.Command) (process.Result, error) {
		writeOutput(t, command, `{"type":"error","sessionID":"ses_new","error":{"data":{"message":"This model's maximum context length is 32768 tokens"}}}`+"\n")
		return process.Result{}, nil
	}}
	implementer, err := NewImplementer(runner, Config{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = implementer.Implement(t.Context(), validImplementationRequest(t))
	var failure *contract.ProviderFailure
	if !errors.As(err, &failure) || failure.Kind != contract.ProviderContextLimit || runner.calls != 1 {
		t.Fatalf("error = %v, calls = %d", err, runner.calls)
	}
}
