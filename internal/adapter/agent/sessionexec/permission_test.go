package sessionexec

import (
	"context"
	"errors"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
	"os"
	"strings"
	"testing"
)

// The capture is reduced from the reported native session: progress followed by
// three rejected module-cache reads, then tool-calls without a terminal answer.
// Session IDs, source paths and unrelated tool payloads are sanitized.
func TestNativePermissionDenialAcrossTeamRoles(t *testing.T) {
	capture, err := os.ReadFile("testdata/opencode-team-denied.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"planning", "implementation", "repair", "review"} {
		t.Run(role, func(t *testing.T) {
			runner := &fakeProcessRunner{run: func(_ context.Context, c process.Command) (process.Result, error) {
				writeOutput(t, c, string(capture))
				return process.Result{}, nil
			}}
			a, _ := NewImplementer(runner, DefaultConfig())
			ro, _ := NewReadOnlyAgent(runner, DefaultConfig())
			req := validRepairRequest(t)
			switch role {
			case "planning":
				_, err = ro.Plan(t.Context(), req.Input)
			case "implementation":
				_, err = a.Implement(t.Context(), validImplementationRequest(t))
			case "repair":
				req.Implementation.AgentSessionID = "ses_team_denied"
				_, err = a.ApplyReview(t.Context(), req)
			case "review":
				_, err = ro.Review(t.Context(), store.ReviewRequest{Input: req.Input, Plan: req.Plan, Implementation: req.Implementation, Validation: req.Validation})
			}
			var denied *store.PermissionDenied
			if !errors.As(err, &denied) || denied.SessionID != "ses_team_denied" || denied.Action.Tool != "read" || denied.Action.Target != "/fixtures/module-cache/experimental.go" || runner.calls != 1 {
				t.Fatal(err, runner.calls)
			}
			if strings.Contains(err.Error(), "JSON") {
				t.Fatal("denial misreported as JSON failure", err)
			}
		})
	}
}

func TestTurnCompletionDistinguishesRecoveryTruncationAndDenial(t *testing.T) {
	capture, err := os.ReadFile("testdata/opencode-team-denied.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, stream            string
		wantDenied, wantSuccess bool
	}{
		{"denied", string(capture), true, false},
		{"ordinary tool failure", strings.ReplaceAll(string(capture), "The user rejected permission to use this specific tool call.", "File not found"), false, false},
		{"tool output cannot spoof denial", `{"type":"tool_use","sessionID":"ses_team_denied","part":{"type":"tool","tool":"read","state":{"status":"completed","output":"The user rejected permission to use this specific tool call."}}}`, false, false},
		{"recovered", string(capture) + successfulEventStream("ses_team_denied", "done"), false, true},
		{"earlier complete result followed by unfinished step", successfulEventStream("ses_team_denied", "earlier") + `{"type":"step_start","sessionID":"ses_team_denied","part":{"type":"step-start"}}`, false, false},
		{"JSON without terminal finish", `{"type":"text","sessionID":"ses_team_denied","part":{"type":"text","text":"{\"schema_version\":\"1\",\"summary\":\"done\",\"changed_files\":[]}"}}`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newEventStream("")
			s.Write([]byte(tc.stream))
			out, err := s.finish()
			var denied *store.PermissionDenied
			if errors.As(err, &denied) != tc.wantDenied || (err == nil) != tc.wantSuccess {
				t.Fatal(out, err)
			}
		})
	}
}
