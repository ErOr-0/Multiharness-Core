package workflow_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type waitFunc func(context.Context, time.Duration) error

func (f waitFunc) Wait(ctx context.Context, d time.Duration) error { return f(ctx, d) }

func providerHarness(t *testing.T, p workflow.ExecutionPolicy, w workflow.RetryWaiter) *workflowHarness {
	t.Helper()
	h := newWorkflowHarness(t)
	service, err := workflow.NewService(workflow.Dependencies{
		Workspace:   h.workspace,
		Planner:     h.planner,
		Implementer: h.implementer,
		Validator:   h.validator,
		Reviewer:    h.reviewer,
		Execution:   p,
		RetryWaiter: w,
		Events:      h.events,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.service = service
	return h
}

func TestTransientPlanningRetriesRespectRetryAfterAndLimits(t *testing.T) {
	for _, failures := range []int{1, 2, 3} {
		waits := 0
		p := workflow.ExecutionPolicy{MaxAgentInvocations: 20, MaxRetries: 2, InitialDelay: time.Millisecond, MaxDelay: 20 * time.Millisecond}
		h := providerHarness(t, p, waitFunc(func(ctx context.Context, d time.Duration) error {
			waits++
			if d < 10*time.Millisecond || d > 20*time.Millisecond {
				t.Errorf("invalid delay %s", d)
			}
			return ctx.Err()
		}))
		calls := 0
		h.planner.run = func(context.Context, store.TaskInput) (store.Plan, error) {
			calls++
			if calls <= failures {
				return store.Plan{}, &store.ProviderFailure{Kind: store.ProviderRateLimited, Attempts: 1, RetryAfterMillis: 10}
			}
			return validPlan(), nil
		}
		result := h.service.Run(t.Context(), validTask(0))
		if calls != min(failures+1, 3) || waits != min(failures, 2) {
			t.Fatalf("calls=%d waits=%d", calls, waits)
		}
		for _, event := range h.events.snapshot() {
			if event.Type == workflow.EventTypeAgentRetryScheduled && (event.RetryDelayMillis < 10 || event.RetryDelayMillis > 20) {
				t.Fatal("retry display delay diverged from bounded wait")
			}
		}
		if failures <= 2 && result.Status != store.TaskStatusApproved {
			t.Fatalf("not approved: %#v", result.Failure)
		}
		if failures == 3 && (result.Status != store.TaskStatusFailed || result.Failure.Provider.Attempts != 3) {
			t.Fatal("retry exhaustion lost failure/attempts")
		}
	}
}

func TestTerminalProviderFailuresNeverRetry(t *testing.T) {
	for _, kind := range []store.ProviderFailureKind{store.ProviderBillingExhausted, store.ProviderAuthentication, store.ProviderAccessDenied, store.ProviderUnknown} {
		h := providerHarness(
			t,
			workflow.ExecutionPolicy{MaxRetries: 10},
			waitFunc(func(context.Context, time.Duration) error { t.Fatal("terminal failure retried"); return nil }),
		)
		h.planner.err = &store.ProviderFailure{Kind: kind, Attempts: 1}
		result := h.service.Run(t.Context(), validTask(3))
		if result.Status != store.TaskStatusFailed || result.Failure.Provider.Kind != kind || result.AgentInvocations != 1 {
			t.Fatal("incorrect terminal failure")
		}
		if len(h.implementer.implementationCalls) != 0 {
			t.Fatal("billing failure advanced pipeline")
		}
	}
}

func TestMutatingStageIsNeverAutomaticallyRetried(t *testing.T) {
	h := providerHarness(
		t,
		workflow.ExecutionPolicy{MaxRetries: 10},
		waitFunc(func(context.Context, time.Duration) error { t.Fatal("implementation was replayed"); return nil }),
	)
	h.implementer.initialErr = &store.ProviderFailure{Kind: store.ProviderOverloaded, Attempts: 1}
	result := h.service.Run(t.Context(), validTask(3))
	if result.Status != store.TaskStatusFailed || result.AgentInvocations != 2 || len(h.implementer.implementationCalls) != 1 {
		t.Fatal("unsafe mutating retry")
	}
}

func TestInvocationBudgetIsEnforcedBeforeEveryAgent(t *testing.T) {
	for _, limit := range []int{1, 2} {
		h := providerHarness(t, workflow.ExecutionPolicy{MaxAgentInvocations: limit}, nil)
		result := h.service.Run(t.Context(), validTask(3))
		if result.Status != store.TaskStatusFailed || result.AgentInvocations != limit || result.Failure.Code != store.FailureCodeInvocationLimit {
			t.Fatalf("budget result=%#v", result)
		}
	}
}

func TestRetryWaitCancellationAndWorkspaceChangesPreventNextCall(t *testing.T) {
	for _, mode := range []string{"cancel", "mutate", "wait error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			var h *workflowHarness
			h = providerHarness(t, workflow.ExecutionPolicy{MaxRetries: 2}, waitFunc(func(context.Context, time.Duration) error {
				switch mode {
				case "cancel":
					cancel()
					return ctx.Err()
				case "mutate":
					h.workspace.session.current.Current.Fingerprint = "changed"
				case "wait error":
					return errors.New("wait failed")
				}
				return nil
			}))
			h.planner.err = &store.ProviderFailure{Kind: store.ProviderRateLimited, Attempts: 1}
			result := h.service.Run(ctx, validTask(0))
			if result.AgentInvocations != 1 {
				t.Fatal("called after cancellation or stale evidence")
			}
			if mode == "cancel" && result.Status != store.TaskStatusCancelled {
				t.Fatal("cancellation lost")
			}
			if mode != "cancel" && result.Status != store.TaskStatusFailed {
				t.Fatal("failure lost")
			}
		})
	}
}

func TestRetryAfterCannotOverflowOrExceedConfiguredWait(t *testing.T) {
	for _, delay := range []int64{math.MaxInt64, 31000} {
		h := providerHarness(
			t,
			workflow.ExecutionPolicy{MaxRetries: 2},
			waitFunc(func(context.Context, time.Duration) error { t.Fatal("wait exceeded configured cap"); return nil }),
		)
		h.planner.err = &store.ProviderFailure{Kind: store.ProviderRateLimited, Attempts: 1, RetryAfterMillis: delay}
		result := h.service.Run(t.Context(), validTask(0))
		if result.Status != store.TaskStatusFailed || result.AgentInvocations != 1 {
			t.Fatal("retried too early")
		}
	}
}

func TestExecutionPolicyRejectsUnsafeConfiguration(t *testing.T) {
	for _, p := range []workflow.ExecutionPolicy{
		{MaxAgentInvocations: -1},
		{MaxAgentInvocations: 10001},
		{MaxRetries: -1},
		{MaxRetries: 11},
		{InitialDelay: -1},
		{InitialDelay: time.Minute, MaxDelay: time.Second},
		{MaxDelay: 25 * time.Hour},
	} {
		h := newWorkflowHarness(t)
		_, err := workflow.NewService(workflow.Dependencies{
			Workspace:   h.workspace,
			Planner:     h.planner,
			Implementer: h.implementer,
			Validator:   h.validator,
			Reviewer:    h.reviewer,
			Execution:   p,
		})
		if err == nil {
			t.Fatalf("accepted unsafe policy %#v", p)
		}
	}
}

func TestUnsafeOrUnclassifiedFailuresAreNotRetried(t *testing.T) {
	var nilFailure *store.ProviderFailure
	for _, cause := range []error{
		nilFailure,
		&store.ProviderFailure{Kind: "untrusted-kind", Attempts: 1},
		errors.New("rate limit exceeded"),
		context.Canceled,
		context.DeadlineExceeded,
	} {
		h := providerHarness(
			t,
			workflow.ExecutionPolicy{MaxRetries: 2},
			waitFunc(func(context.Context, time.Duration) error { t.Fatal("unsafe failure retried"); return nil }),
		)
		h.planner.err = cause
		result := h.service.Run(t.Context(), validTask(0))
		if result.AgentInvocations != 1 || (result.Status != store.TaskStatusFailed && result.Status != store.TaskStatusCancelled) {
			t.Fatalf("unsafe result: %+v", result)
		}
		if err := result.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBackoffIsExponentiallyBoundedAndRetryBudgetCountsLaunches(t *testing.T) {
	waits := 0
	p := workflow.ExecutionPolicy{MaxAgentInvocations: 4, MaxRetries: 10, InitialDelay: 20 * time.Millisecond, MaxDelay: 30 * time.Millisecond}
	h := providerHarness(t, p, waitFunc(func(_ context.Context, delay time.Duration) error {
		ceiling := 30 * time.Millisecond
		if waits == 0 {
			ceiling = 20 * time.Millisecond
		}
		if delay < ceiling/2 || delay > ceiling {
			t.Fatalf("backoff=%v outside equal-jitter range for retry %d", delay, waits+1)
		}
		waits++
		return nil
	}))
	h.planner.err = &store.ProviderFailure{Kind: store.ProviderOverloaded, Attempts: 1}
	result := h.service.Run(t.Context(), validTask(0))
	if result.Status != store.TaskStatusFailed || result.AgentInvocations != 4 || waits != 3 || result.Failure.Provider.Attempts != 4 {
		t.Fatal("retry launch budget or attempt accounting lost")
	}
}

func TestFailedReadOnlyCallCannotMutateBeforeRetry(t *testing.T) {
	h := providerHarness(
		t,
		workflow.ExecutionPolicy{MaxRetries: 2},
		waitFunc(func(context.Context, time.Duration) error { t.Fatal("mutated read-only call retried"); return nil }),
	)
	h.planner.run = func(context.Context, store.TaskInput) (store.Plan, error) {
		h.workspace.session.current.Current.Fingerprint = "changed"
		return store.Plan{}, &store.ProviderFailure{Kind: store.ProviderRateLimited, Attempts: 1}
	}
	result := h.service.Run(t.Context(), validTask(0))
	if result.Status != store.TaskStatusFailed || result.AgentInvocations != 1 {
		t.Fatal("unsafe retry after mutation")
	}
}

func TestInsufficientRemainingDeadlineDoesNotRetryEarly(t *testing.T) {
	p := workflow.ExecutionPolicy{MaxRetries: 2, InitialDelay: 2 * time.Hour, MaxDelay: 4 * time.Hour}
	h := providerHarness(t, p, waitFunc(func(context.Context, time.Duration) error { t.Fatal("wait cannot fit deadline"); return nil }))
	h.planner.err = &store.ProviderFailure{Kind: store.ProviderRateLimited, Attempts: 1}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	result := h.service.Run(ctx, validTask(0))
	if result.Status != store.TaskStatusFailed || result.AgentInvocations != 1 || ctx.Err() != nil {
		t.Fatal("should report provider failure without premature retry or fabricated cancellation")
	}
}
