package workflow

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"multiharness-core/internal/store"
)

// ExecutionPolicy bounds whole-agent launches, not hidden model calls or money.
// Retries are opt-in and permitted only for the read-only planner/reviewer.
type ExecutionPolicy struct {
	MaxAgentInvocations int
	MaxRetries          int
	InitialDelay        time.Duration
	MaxDelay            time.Duration
}

func DefaultExecutionPolicy() ExecutionPolicy {
	return ExecutionPolicy{MaxAgentInvocations: 64, InitialDelay: time.Second, MaxDelay: 30 * time.Second}
}
func (p ExecutionPolicy) withDefaults() ExecutionPolicy {
	d := DefaultExecutionPolicy()
	if p.MaxAgentInvocations == 0 {
		p.MaxAgentInvocations = d.MaxAgentInvocations
	}
	if p.InitialDelay == 0 {
		p.InitialDelay = d.InitialDelay
	}
	if p.MaxDelay == 0 {
		p.MaxDelay = d.MaxDelay
	}
	return p
}
func (p ExecutionPolicy) Validate() error {
	if p.MaxAgentInvocations < 1 || p.MaxAgentInvocations > 10000 {
		return fmt.Errorf("max_agent_invocations must be between 1 and 10000")
	}
	if p.MaxRetries < 0 || p.MaxRetries > 10 {
		return fmt.Errorf("max_retries must be between 0 and 10")
	}
	if p.InitialDelay <= 0 || p.MaxDelay < p.InitialDelay || p.MaxDelay > 24*time.Hour {
		return fmt.Errorf("retry delays must be positive, initial <= maximum, and maximum <= 24h")
	}
	return nil
}

type invocationLimitError struct{}

func (*invocationLimitError) Error() string {
	return "agent invocation limit reached; inspect available work before starting another run"
}

type timerWaiter struct{}

func (timerWaiter) Wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// invokeAgent never automatically replays mutations. An explicitly approved
// billing handoff may continue partial work with the alternate implementation
// port, because Git cannot prove the absence of external side effects.
func invokeAgent[T any](ctx context.Context, service *Service, state *runState, stage store.WorkflowStage, call func(bool) (T, error)) (T, error) {
	var zero T
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		if state.agentInvocations >= service.execution.MaxAgentInvocations {
			return zero, &invocationLimitError{}
		}
		state.agentInvocations++
		result, err := call(state.alternateRoles[roleKey(stage)])
		if err == nil {
			return result, nil
		}
		if ctx.Err() != nil {
			return zero, ctx.Err()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return zero, err
		}
		report := providerFailure(err, attempt)
		if report == nil {
			return zero, err
		}
		if report.Kind == store.ProviderBillingExhausted {
			switched, switchErr := service.authorizeFallback(ctx, state, stage)
			if switchErr != nil {
				return zero, errors.Join(report, switchErr)
			}
			if switched {
				attempt = 0 // Alternate attempt count starts fresh; total launches never reset.
				continue
			}
		}
		if err := service.waitForRetry(ctx, state, stage, report); err != nil {
			return zero, err
		}
	}
}

func providerFailure(err error, attempt int) *store.ProviderFailure {
	var failure *store.ProviderFailure
	if !errors.As(err, &failure) {
		return nil
	}
	report := store.ProviderFailure{Kind: store.ProviderUnknown}
	if failure != nil {
		report = *failure
	}
	report.Attempts = attempt
	if report.Validate() != nil {
		report = store.ProviderFailure{Kind: store.ProviderUnknown, Attempts: attempt}
	}
	return &report
}

// A nil result permits another read-only call. Every other result stops the
// invocation, retaining the provider failure when a retry cannot be authorized.
func (service *Service) waitForRetry(ctx context.Context, state *runState, stage store.WorkflowStage, report *store.ProviderFailure) error {
	if !report.Transient() || report.Attempts > service.execution.MaxRetries ||
		(stage != store.WorkflowStagePlanning && stage != store.WorkflowStageReview) || state.agentInvocations >= service.execution.MaxAgentInvocations {
		return report
	}
	delay, ok := service.execution.retryDelay(report.Attempts, report.RetryAfterMillis)
	if !ok {
		return report
	}
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
		return report
	}
	// Inspect before sleeping AND after waking: evidence must not become stale
	// while waiting, and the original failed read-only call must not mutate.
	if inspectErr := state.inspect(ctx, true); inspectErr != nil {
		return errors.Join(report, inspectErr)
	}
	state.events.publish(Event{
		Type:             EventTypeAgentRetryScheduled,
		Stage:            stage,
		RetryAttempt:     report.Attempts,
		RetryDelayMillis: delay.Milliseconds(),
		ProviderKind:     report.Kind,
		AgentInvocations: state.agentInvocations,
	})
	if err := service.retryWaiter.Wait(ctx, delay); err != nil {
		return err
	}
	if inspectErr := state.inspect(ctx, true); inspectErr != nil {
		return errors.Join(report, inspectErr)
	}
	return nil
}

func (p ExecutionPolicy) retryDelay(attempt int, retryAfterMillis int64) (time.Duration, bool) {
	// Compare before converting to duration so malicious headers cannot overflow.
	if retryAfterMillis > int64(p.MaxDelay/time.Millisecond) {
		return 0, false
	}
	delay := p.InitialDelay
	for i := 1; i < attempt && delay < p.MaxDelay; i++ {
		if delay > p.MaxDelay/2 {
			delay = p.MaxDelay
		} else {
			delay *= 2
		}
	}
	// Equal jitter spreads concurrent customers while preserving a bounded wait.
	delay = delay/2 + time.Duration(rand.Int64N(int64(delay-delay/2)+1))
	if floor := time.Duration(retryAfterMillis) * time.Millisecond; floor > delay {
		delay = floor
	}
	return delay, true
}
