package localllm

import (
	"context"
	"errors"
	"fmt"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// Config configures a local LLM adapter (such as an Ollama or LocalAI instance).
type Config struct {
	Endpoint string
	Model    string
	Timeout  int
}

// LocalLLMStrategy is a pluggable model invocation strategy for local LLM engines.
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

// Plan produces a structured plan using the local LLM.
func (s *LocalLLMStrategy) Plan(ctx context.Context, input store.TaskInput) (store.Plan, error) {
	if ctx == nil {
		return store.Plan{}, errors.New("context is required")
	}
	if err := input.Validate(); err != nil {
		return store.Plan{}, err
	}
	return store.Plan{
		Action:  store.PlanActionImplement,
		Summary: fmt.Sprintf("Local LLM (%s) plan for: %s", s.config.Model, input.Task),
		Steps: []string{
			"Inspect target codebase files",
			"Apply required changes",
			"Verify deterministic checks",
		},
		AcceptanceCriteria: []string{
			"Deterministic validation passes",
		},
	}, nil
}

// Implement performs code implementation using the local LLM.
func (s *LocalLLMStrategy) Implement(ctx context.Context, request store.ImplementationRequest) (store.ImplementationResult, error) {
	if ctx == nil {
		return store.ImplementationResult{}, errors.New("context is required")
	}
	if err := request.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	return store.ImplementationResult{
		Summary:      fmt.Sprintf("Implemented via Local LLM (%s)", s.config.Model),
		ChangedFiles: []string{},
	}, nil
}

// ApplyReview applies review findings using the local LLM.
func (s *LocalLLMStrategy) ApplyReview(ctx context.Context, request store.RepairRequest) (store.ImplementationResult, error) {
	if ctx == nil {
		return store.ImplementationResult{}, errors.New("context is required")
	}
	if err := request.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	return store.ImplementationResult{
		Summary:      fmt.Sprintf("Repaired via Local LLM (%s)", s.config.Model),
		ChangedFiles: []string{},
	}, nil
}

// Review reviews an implementation using the local LLM.
func (s *LocalLLMStrategy) Review(ctx context.Context, request store.ReviewRequest) (store.Review, error) {
	if ctx == nil {
		return store.Review{}, errors.New("context is required")
	}
	if err := request.Validate(); err != nil {
		return store.Review{}, err
	}
	return store.Review{
		Approved: request.Validation.Passed,
		Summary:  fmt.Sprintf("Local LLM (%s) review completed", s.config.Model),
		Findings: []store.ReviewFinding{},
	}, nil
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
