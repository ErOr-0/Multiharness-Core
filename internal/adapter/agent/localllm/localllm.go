package localllm

import (
	"context"
	"errors"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// ErrUnavailable prevents this unfinished adapter from manufacturing evidence.
var ErrUnavailable = errors.New("local LLM integration is not implemented; configure a supported Codex or OpenCode adapter")

// Config configures a local LLM adapter (such as an Ollama or LocalAI instance).
type Config struct {
	Endpoint string
	Model    string
	Timeout  int
}

// LocalLLMStrategy reserves an adapter boundary for future local LLM integration.
// Every operation fails explicitly until actual invocation/review is implemented.
type LocalLLMStrategy struct {
	config Config
}

// NewLocalLLMStrategy creates an instance of LocalLLMStrategy.
func NewLocalLLMStrategy(config Config) (*LocalLLMStrategy, error) {
	if config.Model == "" {
		config.Model = "llama3"
	}
	return &LocalLLMStrategy{config: config}, nil
}

// Plan fails closed; no local LLM has been integrated.
func (s *LocalLLMStrategy) Plan(ctx context.Context, input store.TaskInput) (store.Plan, error) {
	if ctx == nil {
		return store.Plan{}, errors.New("context is required")
	}
	if err := input.Validate(); err != nil {
		return store.Plan{}, err
	}
	if err := ctx.Err(); err != nil {
		return store.Plan{}, err
	}
	return store.Plan{}, ErrUnavailable
}

// Implement fails closed without claiming to change files.
func (s *LocalLLMStrategy) Implement(ctx context.Context, request store.ImplementationRequest) (store.ImplementationResult, error) {
	if ctx == nil {
		return store.ImplementationResult{}, errors.New("context is required")
	}
	if err := request.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return store.ImplementationResult{}, err
	}
	return store.ImplementationResult{}, ErrUnavailable
}

// ApplyReview fails closed without claiming to repair files.
func (s *LocalLLMStrategy) ApplyReview(ctx context.Context, request store.RepairRequest) (store.ImplementationResult, error) {
	if ctx == nil {
		return store.ImplementationResult{}, errors.New("context is required")
	}
	if err := request.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return store.ImplementationResult{}, err
	}
	return store.ImplementationResult{}, ErrUnavailable
}

// Review fails closed, including when all deterministic checks passed.
func (s *LocalLLMStrategy) Review(ctx context.Context, request store.ReviewRequest) (store.Review, error) {
	if ctx == nil {
		return store.Review{}, errors.New("context is required")
	}
	if err := request.Validate(); err != nil {
		return store.Review{}, err
	}
	if err := ctx.Err(); err != nil {
		return store.Review{}, err
	}
	return store.Review{}, ErrUnavailable
}

// Ensure LocalLLMStrategy satisfies all workflow strategy interfaces.
var (
	_ workflow.PlanningStrategy       = (*LocalLLMStrategy)(nil)
	_ workflow.ImplementationStrategy = (*LocalLLMStrategy)(nil)
	_ workflow.ReviewStrategy         = (*LocalLLMStrategy)(nil)
	_ workflow.Planner                = (*LocalLLMStrategy)(nil)
	_ workflow.Implementer            = (*LocalLLMStrategy)(nil)
	_ workflow.Reviewer               = (*LocalLLMStrategy)(nil)
)
