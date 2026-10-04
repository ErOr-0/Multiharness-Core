// Package delegation owns the single-agent use case. Provider protocols,
// configuration loading and terminal rendering belong to outer modules.
package delegation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"multiharness-core/internal/contract"
)

// Agent executes exactly one native CLI turn. Partial output must survive errors.
type Agent interface {
	Execute(context.Context, contract.TaskInput) (contract.DirectResponse, error)
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

func (s *Service) Run(ctx context.Context, input contract.TaskInput) contract.TaskOutput {
	out := contract.TaskOutput{Status: contract.TaskStatusFailed, Summary: "Agent could not start"}
	fail := func(code contract.FailureCode, err error) contract.TaskOutput {
		out.Failure = &contract.TaskFailure{Stage: contract.WorkflowStageDelegation, Code: code, Message: err.Error()}
		var provider *contract.ProviderFailure
		if errors.As(err, &provider) && provider.Validate() == nil {
			out.Failure.Provider = provider
		}
		return out
	}
	if ctx == nil {
		return fail(contract.FailureCodeInvalidInput, errors.New("context is required"))
	}
	if err := input.Validate(); err != nil {
		return fail(contract.FailureCodeInvalidInput, err)
	}
	if ctx.Err() != nil {
		out.Status = contract.TaskStatusCancelled
		out.Summary = "Cancelled before the agent started"
		return out
	}
	run, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	out.AgentInvocations = 1
	response, err := s.agent.Execute(run, input)
	out.Direct = &response
	if run.Err() != nil || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		out.Status = contract.TaskStatusCancelled
		out.Summary = "Agent cancelled; any edits and partial response were kept"
		if errors.Is(run.Err(), context.DeadlineExceeded) {
			out.Status = contract.TaskStatusTimedOut
			out.Summary = fmt.Sprintf("Agent stopped: %s deadline (%s) expired; any edits and partial response were kept", s.timeoutName, s.timeout)
		} else if errors.Is(err, context.DeadlineExceeded) {
			out.Status = contract.TaskStatusTimedOut
			out.Summary = "Agent stopped: the CLI or provider reported a deadline; the Multiharness deadline did not expire"
		}
		return out
	}
	if err != nil {
		out.Summary = "Agent failed; any edits and partial response were kept"
		return fail(contract.FailureCodeAgent, err)
	}
	if response.NeedsInput {
		out.Status = contract.TaskStatusNeedsInput
		out.Summary = "The CLI needs permission or input before it can continue"
		if response.Blocked != nil {
			out.Summary = fmt.Sprintf("The CLI rejected permission for %s", response.Blocked.Tool)
			if response.Blocked.Target != "" {
				out.Summary += fmt.Sprintf(" on %q", response.Blocked.Target)
			}
			out.Summary += "; any edits and the conversation were kept. Ask the agent to continue without that action, or allow the specific action in the CLI's permission settings and retry."
		}
		return out
	}
	if strings.TrimSpace(response.Text) == "" {
		return fail(contract.FailureCodeInvalidOutput, errors.New("CLI ended without a final response"))
	}
	out.Status, out.Summary = contract.TaskStatusResponded, response.Text
	return out
}
