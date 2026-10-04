package structured

import (
	"context"
	"errors"

	"multiharness-core/internal/contract"
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
	Budget      Budget
}

func (a Agent) budget() Budget { return a.Budget.withDefaults() }

func (a Agent) outputError(role, session string, err error) error {
	if err == nil {
		return nil
	}
	if a.OutputError != nil {
		return a.OutputError(role, session, err)
	}
	return &OutputError{Role: role, Cause: err}
}
func (a Agent) Plan(ctx context.Context, input contract.TaskInput) (contract.Plan, error) {
	if a.CanWrite {
		return contract.Plan{}, errors.New("planning requires read-only execution")
	}
	if err := input.Validate(); err != nil {
		return contract.Plan{}, err
	}
	prompt, err := PlanningPromptWithBudget(input, a.budget())
	if err != nil {
		return contract.Plan{}, err
	}
	role := "planning"
	if input.AnswerOnly {
		role = "answering"
	}
	response, err := a.Execute(ctx, Invocation{Role: role, WorkingDir: input.WorkingDir, Prompt: prompt, Schema: PlanSchema()})
	if err != nil {
		return contract.Plan{}, err
	}
	result, err := ParsePlan(response.Data)
	return result, a.outputError(role, response.SessionID, err)
}
func (a Agent) Review(ctx context.Context, request contract.ReviewRequest) (contract.Review, error) {
	if a.CanWrite {
		return contract.Review{}, errors.New("review requires read-only execution")
	}
	if err := request.Validate(); err != nil {
		return contract.Review{}, err
	}
	prompt, err := ReviewPromptWithBudget(request, a.budget())
	if err != nil {
		return contract.Review{}, err
	}
	response, err := a.Execute(ctx, Invocation{Role: "review", WorkingDir: request.Input.WorkingDir, Prompt: prompt, Schema: ReviewSchema()})
	if err != nil {
		return contract.Review{}, err
	}
	result, err := ParseReview(response.Data)
	return result, a.outputError("review", response.SessionID, err)
}
func (a Agent) Implement(ctx context.Context, request contract.ImplementationRequest) (contract.ImplementationResult, error) {
	if err := request.Validate(); err != nil {
		return contract.ImplementationResult{}, err
	}
	prompt, err := ImplementationPromptWithBudget(request, a.budget())
	if err != nil {
		return contract.ImplementationResult{}, err
	}
	return a.implement(ctx, "implementation", request.Input.WorkingDir, request.Input.SessionID, prompt)
}
func (a Agent) ApplyReview(ctx context.Context, request contract.RepairRequest) (contract.ImplementationResult, error) {
	if err := request.Validate(); err != nil {
		return contract.ImplementationResult{}, err
	}
	if !a.Resume {
		request.Implementation.AgentSessionID = ""
	}
	prompt, err := RepairPromptWithBudget(request, a.budget())
	if err != nil {
		return contract.ImplementationResult{}, err
	}
	session := request.Implementation.AgentSessionID
	result, err := a.implement(ctx, "repair", request.Input.WorkingDir, session, prompt)
	// A resumed session carries the whole implementation transcript, which can
	// outgrow a small model's context across repair rounds. The repair handoff
	// is self-contained, so it continues once in a fresh session.
	var failure *contract.ProviderFailure
	if session != "" && errors.As(err, &failure) && failure.Kind == contract.ProviderContextLimit && ctx.Err() == nil {
		return a.implement(ctx, "repair", request.Input.WorkingDir, "", prompt)
	}
	return result, err
}

// ReviewChunk reviews one bounded diff chunk. The workflow drives one call per
// chunk and keeps the workspace fingerprint stable across calls.
func (a Agent) ReviewChunk(ctx context.Context, request contract.ReviewRequest, chunk contract.ReviewChunk) (contract.Review, error) {
	if a.CanWrite {
		return contract.Review{}, errors.New("review requires read-only execution")
	}
	if err := request.Validate(); err != nil {
		return contract.Review{}, err
	}
	prompt, err := ReviewChunkPrompt(request, chunk, a.budget())
	if err != nil {
		return contract.Review{}, err
	}
	response, err := a.Execute(ctx, Invocation{Role: "review", WorkingDir: request.Input.WorkingDir, Prompt: prompt, Schema: ReviewSchema()})
	if err != nil {
		return contract.Review{}, err
	}
	result, err := ParseReview(response.Data)
	return result, a.outputError("review", response.SessionID, err)
}

// ReviewSynthesis aggregates chunk findings without resending every diff.
func (a Agent) ReviewSynthesis(ctx context.Context, request contract.ReviewRequest, findings []contract.ReviewFinding, summaries []string) (contract.Review, error) {
	if a.CanWrite {
		return contract.Review{}, errors.New("review requires read-only execution")
	}
	if err := request.Validate(); err != nil {
		return contract.Review{}, err
	}
	prompt, err := ReviewSynthesisPrompt(request, findings, summaries, a.budget())
	if err != nil {
		return contract.Review{}, err
	}
	response, err := a.Execute(ctx, Invocation{Role: "review", WorkingDir: request.Input.WorkingDir, Prompt: prompt, Schema: ReviewSchema()})
	if err != nil {
		return contract.Review{}, err
	}
	result, err := ParseReview(response.Data)
	return result, a.outputError("review", response.SessionID, err)
}

func (a Agent) implement(ctx context.Context, role, dir, session, prompt string) (contract.ImplementationResult, error) {
	if !a.CanWrite {
		return contract.ImplementationResult{}, errors.New("implementation requires write execution")
	}
	if !a.Resume {
		session = ""
	}
	prompt += "\nInspect current and partial work before continuing; do not blindly replay completed changes or external side effects. Earlier validation/findings describe the previous completed round, not proof about newer partial edits."
	response, err := a.Execute(ctx, Invocation{Role: role, WorkingDir: dir, Prompt: prompt, Schema: ImplementationSchema(), SessionID: session})
	if err != nil {
		return contract.ImplementationResult{}, err
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
