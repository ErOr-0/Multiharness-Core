package main

import (
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/adapter/setup"
	validationadapter "multiharness-core/internal/adapter/validation"
	gitworkspace "multiharness-core/internal/adapter/workspace/git"
	"multiharness-core/internal/config"
	"multiharness-core/internal/workflow"
)

// The same composition is used by production and opt-in integration tests.
// Tests may decorate a port to inject a reproducible fault, never agent output.
func buildDependenciesWithInstallation(cfg config.Config, events workflow.EventSink, confirm setup.Confirmation) (workflow.Dependencies, error) {
	runner := process.NewOSRunner()
	agents, installation := buildAgentRunners(cfg, events, runner, confirm)
	workspace, err := gitworkspace.NewWorkspace(setup.Runner{Runner: runner, Tool: "git", Manager: installation}, cfg.Git.Adapter())
	if err != nil {
		return workflow.Dependencies{}, err
	}
	validator, err := validationadapter.NewValidator(runner, cfg.Validation.Adapter())
	if err != nil {
		return workflow.Dependencies{}, err
	}
	dependencies := workflow.Dependencies{Workspace: workspace, Validator: validator, Events: events, Execution: cfg.Execution.Policy()}
	if err := agents.composePlanning(cfg, &dependencies); err != nil {
		return workflow.Dependencies{}, err
	}
	if err := agents.composeImplementation(cfg, &dependencies); err != nil {
		return workflow.Dependencies{}, err
	}
	if err := agents.composeReview(cfg, &dependencies); err != nil {
		return workflow.Dependencies{}, err
	}
	return dependencies, nil
}
