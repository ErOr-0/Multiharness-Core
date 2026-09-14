// Package delegation owns the single-agent use case. Provider protocols,
// configuration loading and terminal rendering belong to outer modules.
package delegation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"multiharness-core/internal/store"
)

// Agent executes exactly one native CLI turn. Partial output must survive errors.
type Agent interface {
	Execute(context.Context, store.TaskInput) (store.DirectResponse, error)
}

type Service struct {
	agent       Agent
	timeout     time.Duration
	timeoutName string
}

func NewService(agent Agent, timeout time.Duration, timeoutName string) (*Service, error) {
	if agent == nil || timeout <= 0 || (timeoutName != "timeout" && timeoutName != "implementer-timeout") {
		return nil, errors.New("delegation requires an agent and a named positive deadline")
	}
	return &Service{agent: agent, timeout: timeout, timeoutName: timeoutName}, nil
}

func (s *Service) Run(ctx context.Context, input store.TaskInput) store.TaskOutput {
	out := store.TaskOutput{Status: store.TaskStatusFailed, Summary: "Agent could not start"}
	fail := func(code store.FailureCode, err error) store.TaskOutput {
		out.Failure = &store.TaskFailure{Stage: store.WorkflowStageDelegation, Code: code, Message: err.Error()}
		var provider *store.ProviderFailure
		if errors.As(err, &provider) && provider.Validate() == nil {
			out.Failure.Provider = provider
		}
		return out
	}
	if ctx == nil {
		return fail(store.FailureCodeInvalidInput, errors.New("context is required"))
	}
	if err := input.Validate(); err != nil {
		return fail(store.FailureCodeInvalidInput, err)
	}
	if ctx.Err() != nil {
		out.Status = store.TaskStatusCancelled
		out.Summary = "Cancelled before the agent started"
		return out
	}
	run, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	out.AgentInvocations = 1
	response, err := s.agent.Execute(run, input)
	out.Direct = &response
	if run.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		out.Status = store.TaskStatusCancelled
		out.Summary = "Agent cancelled; any edits and partial response were kept"
		if errors.Is(run.Err(), context.DeadlineExceeded) {
			out.Status = store.TaskStatusTimedOut
			out.Summary = fmt.Sprintf("Agent stopped: %s deadline (%s) expired; any edits and partial response were kept", s.timeoutName, s.timeout)
		} else if errors.Is(err, context.DeadlineExceeded) {
			out.Status = store.TaskStatusTimedOut
			out.Summary = "Agent stopped: the CLI or provider reported a deadline; the Multiharness deadline did not expire"
		}
		return out
	}
	if err != nil {
		out.Summary = "Agent failed; any edits and partial response were kept"
		return fail(store.FailureCodeAgent, err)
	}
	if response.NeedsInput {
		out.Status = store.TaskStatusNeedsInput
		out.Summary = "The CLI needs permission or input before it can continue"
		return out
	}
	if strings.TrimSpace(response.Text) == "" {
		return fail(store.FailureCodeInvalidOutput, errors.New("CLI ended without a final response"))
	}
	out.Status, out.Summary = store.TaskStatusResponded, response.Text
	return out
}
