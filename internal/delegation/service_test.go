package delegation_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/delegation"
)

type agentFunc func(context.Context, contract.TaskInput) (contract.DirectResponse, error)

func (f agentFunc) Execute(ctx context.Context, in contract.TaskInput) (contract.DirectResponse, error) {
	return f(ctx, in)
}

func TestDirectTurnPreservesPromptAndDoesNotInventReviewEvidence(t *testing.T) {
	calls := 0
	agent := agentFunc(func(_ context.Context, in contract.TaskInput) (contract.DirectResponse, error) {
		calls++
		if in.Task != "please implement this" || in.SessionID != "ses_1" {
			t.Fatalf("changed request: %+v", in)
		}
		return contract.DirectResponse{Text: "Which framework version should I use?", SessionID: "ses_1"}, nil
	})
	s, err := delegation.NewService(agent, time.Minute, "timeout")
	if err != nil {
		t.Fatal(err)
	}
	out := s.Run(t.Context(), contract.TaskInput{Task: "please implement this", WorkingDir: "/workspace", SessionID: "ses_1"})
	if calls != 1 || out.Status != contract.TaskStatusResponded || out.Plan != nil || out.Validation != nil || out.LastReview != nil {
		t.Fatalf("unexpected result: %+v", out)
	}
	if err := out.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestInterruptionsKeepPartialResponseAndNeverRetry(t *testing.T) {
	for _, kind := range []string{"deadline", "provider-deadline", "cancelled", "provider-error", "input"} {
		t.Run(kind, func(t *testing.T) {
			calls := 0
			agent := agentFunc(func(ctx context.Context, _ contract.TaskInput) (contract.DirectResponse, error) {
				calls++
				r := contract.DirectResponse{Text: "File edited", SessionID: "ses_partial"}
				switch kind {
				case "deadline":
					<-ctx.Done()
					return r, ctx.Err()
				case "provider-deadline":
					return r, context.DeadlineExceeded
				case "cancelled":
					return r, context.Canceled
				case "input":
					r.NeedsInput = true
					return r, nil
				default:
					return r, errors.New("provider failed")
				}
			})
			timeout := time.Minute
			if kind == "deadline" {
				timeout = time.Millisecond
			}
			s, _ := delegation.NewService(agent, timeout, "implementer-timeout")
			out := s.Run(t.Context(), contract.TaskInput{Task: "implement", WorkingDir: "/workspace"})
			want := map[string]contract.TaskStatus{"deadline": contract.TaskStatusTimedOut, "provider-deadline": contract.TaskStatusTimedOut, "cancelled": contract.TaskStatusCancelled, "provider-error": contract.TaskStatusFailed, "input": contract.TaskStatusNeedsInput}[kind]
			if calls != 1 || out.Status != want || out.Direct.Text != "File edited" || out.Direct.SessionID != "ses_partial" {
				t.Fatalf("lost outcome: %+v", out)
			}
			if kind == "deadline" && !strings.Contains(out.Summary, "implementer-timeout") {
				t.Fatal(out.Summary)
			}
			if err := out.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInvalidAndCancelledRequestsDoNotInvokeAgent(t *testing.T) {
	s, _ := delegation.NewService(agentFunc(func(context.Context, contract.TaskInput) (contract.DirectResponse, error) {
		t.Fatal("agent invoked")
		return contract.DirectResponse{}, nil
	}), time.Minute, "timeout")
	if out := s.Run(t.Context(), contract.TaskInput{}); out.Status != contract.TaskStatusFailed {
		t.Fatal(out)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if out := s.Run(ctx, contract.TaskInput{Task: "task", WorkingDir: "/workspace"}); out.Status != contract.TaskStatusCancelled {
		t.Fatal(out)
	}
}
