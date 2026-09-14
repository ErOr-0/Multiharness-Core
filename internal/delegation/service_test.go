package delegation_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/delegation"
	"multiharness-core/internal/store"
)

type agentFunc func(context.Context, store.TaskInput) (store.DirectResponse, error)

func (f agentFunc) Execute(ctx context.Context, in store.TaskInput) (store.DirectResponse, error) {
	return f(ctx, in)
}

func TestDirectTurnPreservesPromptAndDoesNotInventReviewEvidence(t *testing.T) {
	calls := 0
	agent := agentFunc(func(_ context.Context, in store.TaskInput) (store.DirectResponse, error) {
		calls++
		if in.Task != "please implement this" || in.SessionID != "ses_1" {
			t.Fatalf("changed request: %+v", in)
		}
		return store.DirectResponse{Text: "Which framework version should I use?", SessionID: "ses_1"}, nil
	})
	s, err := delegation.NewService(agent, time.Minute, "timeout")
	if err != nil {
		t.Fatal(err)
	}
	out := s.Run(t.Context(), store.TaskInput{Task: "please implement this", WorkingDir: "/workspace", SessionID: "ses_1"})
	if calls != 1 || out.Status != store.TaskStatusResponded || out.Plan != nil || out.Validation != nil || out.LastReview != nil {
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
			agent := agentFunc(func(ctx context.Context, _ store.TaskInput) (store.DirectResponse, error) {
				calls++
				r := store.DirectResponse{Text: "File edited", SessionID: "ses_partial"}
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
			out := s.Run(t.Context(), store.TaskInput{Task: "implement", WorkingDir: "/workspace"})
			want := map[string]store.TaskStatus{"deadline": store.TaskStatusTimedOut, "provider-deadline": store.TaskStatusTimedOut, "cancelled": store.TaskStatusCancelled, "provider-error": store.TaskStatusFailed, "input": store.TaskStatusNeedsInput}[kind]
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
	s, _ := delegation.NewService(agentFunc(func(context.Context, store.TaskInput) (store.DirectResponse, error) {
		t.Fatal("agent invoked")
		return store.DirectResponse{}, nil
	}), time.Minute, "timeout")
	if out := s.Run(t.Context(), store.TaskInput{}); out.Status != store.TaskStatusFailed {
		t.Fatal(out)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if out := s.Run(ctx, store.TaskInput{Task: "task", WorkingDir: "/workspace"}); out.Status != store.TaskStatusCancelled {
		t.Fatal(out)
	}
}
