package structured

import (
	"context"
	"errors"

	"multiharness-core/internal/contract"
)

// Invocation is the shared structured-role boundary. Protocol adapters own CLI
// arguments, permissions and response envelopes.
type Invocation struct {
	Role, WorkingDir, Prompt string
	Schema                   []byte
}
type Response struct {
	Data []byte
}
type ExecuteFunc func(context.Context, Invocation) (Response, error)

// Agent implements the role contracts once for every harness. Each invocation
// starts a fresh agent process and receives its full context in the prompt.
type Agent struct {
	Execute     ExecuteFunc
	CanWrite    bool
	OutputError func(role string, err error) error
	Budget      Budget
}

func (a Agent) budget() Budget { return a.Budget.withDefaults() }

func (a Agent) outputError(role string, err error) error {
	if err == nil {
		return nil
	}
	if a.OutputError != nil {
		return a.OutputError(role, err)
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
	return result, a.outputError(role, err)
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
	return result, a.outputError("review", err)
}
func (a Agent) Implement(ctx context.Context, request contract.ImplementationRequest) (contract.ImplementationResult, error) {
	if err := request.Validate(); err != nil {
		return contract.ImplementationResult{}, err
	}
	prompt, err := ImplementationPromptWithBudget(request, a.budget())
	if err != nil {
		return contract.ImplementationResult{}, err
	}
	return a.implement(ctx, "implementation", request.Input.WorkingDir, prompt)
}
func (a Agent) ApplyReview(ctx context.Context, request contract.RepairRequest) (contract.ImplementationResult, error) {
	if err := request.Validate(); err != nil {
		return contract.ImplementationResult{}, err
	}
	prompt, err := RepairPromptWithBudget(request, a.budget())
	if err != nil {
		return contract.ImplementationResult{}, err
	}
	return a.implement(ctx, "repair", request.Input.WorkingDir, prompt)
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
	return result, a.outputError("review", err)
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
	return result, a.outputError("review", err)
}

func (a Agent) implement(ctx context.Context, role, dir, prompt string) (contract.ImplementationResult, error) {
	if !a.CanWrite {
		return contract.ImplementationResult{}, errors.New("implementation requires write execution")
	}
	prompt += "\nInspect current and partial work before continuing; do not blindly replay completed changes or external side effects. Earlier validation/findings describe the previous completed round, not proof about newer partial edits."
	response, err := a.Execute(ctx, Invocation{Role: role, WorkingDir: dir, Prompt: prompt, Schema: ImplementationSchema()})
	if err != nil {
		return contract.ImplementationResult{}, err
	}
	result, err := ParseImplementation(response.Data)
	return result, a.outputError(role, err)
}
