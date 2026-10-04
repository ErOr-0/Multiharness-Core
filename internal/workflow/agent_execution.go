package workflow

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"multiharness-core/internal/contract"
)

type ExecutionPolicy struct {
	MaxAgentInvocations int
	MaxRetries          int
	InitialDelay        time.Duration
	MaxDelay            time.Duration
	MaxPromptBytes      int
	ReviewChunkBytes    int
}

func DefaultExecutionPolicy() ExecutionPolicy {
	return ExecutionPolicy{MaxAgentInvocations: 64, InitialDelay: time.Second, MaxDelay: 30 * time.Second, MaxPromptBytes: 262144, ReviewChunkBytes: 131072}
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
	if p.MaxPromptBytes == 0 {
		p.MaxPromptBytes = d.MaxPromptBytes
	}
	if p.ReviewChunkBytes == 0 {
		p.ReviewChunkBytes = d.ReviewChunkBytes
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
	if p.MaxPromptBytes <= 0 || p.ReviewChunkBytes <= 0 {
		return fmt.Errorf("max_prompt_bytes and review_chunk_bytes must be positive")
	}
	if p.ReviewChunkBytes > p.MaxPromptBytes {
		return fmt.Errorf("review_chunk_bytes must not exceed max_prompt_bytes")
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

func invokeAgent[T any](ctx context.Context, service *Service, state *runState, stage contract.WorkflowStage, call func(bool) (T, error)) (T, error) {
	var zero T
	interruptedReviewRetries := 0
	permissionRetries := 0

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

		if errors.Is(err, context.Canceled) {
			// A reviewer is read-only, so a locally interrupted invocation can
			// be retried once. The parent context was checked above; a real
			// workflow cancellation must never launch another agent.
			if stage == contract.WorkflowStageReview && interruptedReviewRetries == 0 &&
				state.agentInvocations < service.execution.MaxAgentInvocations {
				if inspectErr := state.inspectAcquired(ctx, true); inspectErr != nil {
					return zero, errors.Join(err, inspectErr)
				}
				delay := service.execution.InitialDelay
				if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
					return zero, err
				}
				interruptedReviewRetries++
				state.events.publish(Event{
					Type: EventTypeAgentRetryScheduled, Stage: stage,
					RetryAttempt: interruptedReviewRetries, RetryDelayMillis: delay.Milliseconds(),
					ProviderKind: contract.ProviderUnknown, AgentInvocations: state.agentInvocations,
				})
				if waitErr := service.retryWaiter.Wait(ctx, delay); waitErr != nil {
					return zero, waitErr
				}
				if inspectErr := state.inspectAcquired(ctx, true); inspectErr != nil {
					return zero, errors.Join(err, inspectErr)
				}
				continue
			}
			return zero, fmt.Errorf("%s agent stopped while workflow remained active: %w", stage, err)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return zero, err
		}

		if denied := permissionOnly(err); denied != nil && denied.Validate() == nil {
			if denied.UserDeclined || service.permissionResolver == nil || permissionRetries >= 3 || state.agentInvocations >= service.execution.MaxAgentInvocations {
				return zero, err
			}
			// Capture partial writes before waiting; keep the original baseline
			// and lease so a retry cannot reclassify our edits as user work.
			readOnly := stage != contract.WorkflowStageImplementation && stage != contract.WorkflowStageRepair
			if inspectErr := state.inspectAcquired(ctx, readOnly); inspectErr != nil {
				return zero, errors.Join(err, inspectErr)
			}
			retry, resolveErr := service.permissionResolver.ResolvePermission(ctx, stage, *denied)
			if ctx.Err() != nil {
				return zero, ctx.Err()
			}
			if resolveErr != nil {
				return zero, errors.Join(err, resolveErr)
			}
			if !retry {
				return zero, err
			}
			if inspectErr := state.inspectAcquired(ctx, true); inspectErr != nil {
				return zero, errors.Join(err, inspectErr)
			}
			permissionRetries++
			continue
		}

		report := providerFailure(err, attempt)
		if report == nil {
			return zero, err
		}
		if report.Kind == contract.ProviderBillingExhausted {
			switched, switchErr := service.authorizeFallback(ctx, state, stage)
			if switchErr != nil {
				return zero, errors.Join(report, switchErr)
			}
			if switched {
				attempt = 0
				continue
			}
		}
		if err := service.waitForRetry(ctx, state, stage, report); err != nil {
			return zero, err
		}
	}
}

func providerFailure(err error, attempt int) *contract.ProviderFailure {
	var failure *contract.ProviderFailure
	if !errors.As(err, &failure) {
		return nil
	}
	report := contract.ProviderFailure{Kind: contract.ProviderUnknown}
	if failure != nil {
		report = *failure
	}
	report.Attempts = attempt
	if report.Validate() != nil {
		report = contract.ProviderFailure{Kind: contract.ProviderUnknown, Attempts: attempt}
	}
	return &report
}

func (service *Service) waitForRetry(ctx context.Context, state *runState, stage contract.WorkflowStage, report *contract.ProviderFailure) error {
	if !report.Transient() || report.Attempts > service.execution.MaxRetries ||
		(stage != contract.WorkflowStagePlanning && stage != contract.WorkflowStageReview) || state.agentInvocations >= service.execution.MaxAgentInvocations {
		return report
	}
	delay, ok := service.execution.retryDelay(report.Attempts, report.RetryAfterMillis)
	if !ok {
		return report
	}

	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) <= delay {
		return report
	}

	if inspectErr := state.inspectAcquired(ctx, true); inspectErr != nil {
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
	if inspectErr := state.inspectAcquired(ctx, true); inspectErr != nil {
		return errors.Join(report, inspectErr)
	}
	return nil
}

func (p ExecutionPolicy) retryDelay(attempt int, retryAfterMillis int64) (time.Duration, bool) {

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

	delay = delay/2 + time.Duration(rand.Int64N(int64(delay-delay/2)+1))
	if floor := time.Duration(retryAfterMillis) * time.Millisecond; floor > delay {
		delay = floor
	}

	return delay, true
}
