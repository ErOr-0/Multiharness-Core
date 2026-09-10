package structured

import (
	"context"
	"errors"

	"multiharness-core/internal/store"
)

// Invocation is the shared structured-role boundary. Protocol adapters own CLI
// arguments, permissions, response envelopes and validation of session identifiers.
type Invocation struct {
	Role, WorkingDir, Prompt, SessionID string
	Schema                              []byte
}
type Response struct {
	Data      []byte
	SessionID string
}
type ExecuteFunc func(context.Context, Invocation) (Response, error)

// Agent implements role contracts once for both schema and session protocols.
// Fresh invocations receive full context; only session protocols opt into resume.
type Agent struct {
	Execute     ExecuteFunc
	CanWrite    bool
	Resume      bool
	OutputError func(role, session string, err error) error
}

func (a Agent) outputError(role, session string, err error) error {
	if err == nil {
		return nil
	}
	if a.OutputError != nil {
		return a.OutputError(role, session, err)
	}
	return &OutputError{Role: role, Cause: err}
}
func (a Agent) Plan(ctx context.Context, input store.TaskInput) (store.Plan, error) {
	if a.CanWrite {
		return store.Plan{}, errors.New("planning requires read-only execution")
	}
	if err := input.Validate(); err != nil {
		return store.Plan{}, err
	}
	prompt, err := PlanningPrompt(input)
	if err != nil {
		return store.Plan{}, err
	}
	response, err := a.Execute(ctx, Invocation{Role: "planning", WorkingDir: input.WorkingDir, Prompt: prompt, Schema: PlanSchema()})
	if err != nil {
		return store.Plan{}, err
	}
	result, err := ParsePlan(response.Data)
	return result, a.outputError("planning", response.SessionID, err)
}
func (a Agent) Review(ctx context.Context, request store.ReviewRequest) (store.Review, error) {
	if a.CanWrite {
		return store.Review{}, errors.New("review requires read-only execution")
	}
	if err := request.Validate(); err != nil {
		return store.Review{}, err
	}
	prompt, err := ReviewPrompt(request)
	if err != nil {
		return store.Review{}, err
	}
	response, err := a.Execute(ctx, Invocation{Role: "review", WorkingDir: request.Input.WorkingDir, Prompt: prompt, Schema: ReviewSchema()})
	if err != nil {
		return store.Review{}, err
	}
	result, err := ParseReview(response.Data)
	return result, a.outputError("review", response.SessionID, err)
}
func (a Agent) Implement(ctx context.Context, request store.ImplementationRequest) (store.ImplementationResult, error) {
	if err := request.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	prompt, err := ImplementationPrompt(request)
	if err != nil {
		return store.ImplementationResult{}, err
	}
	return a.implement(ctx, "implementation", request.Input.WorkingDir, request.Input.SessionID, prompt)
}
func (a Agent) ApplyReview(ctx context.Context, request store.RepairRequest) (store.ImplementationResult, error) {
	if err := request.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	if !a.Resume {
		request.Implementation.AgentSessionID = ""
	}
	prompt, err := RepairPrompt(request)
	if err != nil {
		return store.ImplementationResult{}, err
	}
	return a.implement(ctx, "repair", request.Input.WorkingDir, request.Implementation.AgentSessionID, prompt)
}
func (a Agent) implement(ctx context.Context, role, dir, session, prompt string) (store.ImplementationResult, error) {
	if !a.CanWrite {
		return store.ImplementationResult{}, errors.New("implementation requires write execution")
	}
	if !a.Resume {
		session = ""
	}
	prompt += "\nInspect current and partial work before continuing; do not blindly replay completed changes or external side effects. Earlier validation/findings describe the previous completed round, not proof about newer partial edits."
	response, err := a.Execute(ctx, Invocation{Role: role, WorkingDir: dir, Prompt: prompt, Schema: ImplementationSchema(), SessionID: session})
	if err != nil {
		return store.ImplementationResult{}, err
	}
	result, err := ParseImplementation(response.Data)
	if err != nil {
		return result, a.outputError(role, response.SessionID, err)
	}
	if a.Resume {
		result.AgentSessionID = response.SessionID
	}
	return result, nil
}
