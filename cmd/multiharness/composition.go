package main

import (
	"fmt"
	"time"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/adapter/agent/schemaexec"
	"multiharness-core/internal/adapter/agent/sessionexec"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/adapter/setup"
	validationadapter "multiharness-core/internal/adapter/validation"
	folderworkspace "multiharness-core/internal/adapter/workspace/folder"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// The same composition is used by production and opt-in integration tests.
// Tests may decorate a port to inject a reproducible fault, never agent output.
func buildDependenciesWithInstallation(cfg config.Config, events workflow.EventSink, confirm setup.Confirmation) (workflow.Dependencies, error) {
	return buildDependenciesWithApprovals(cfg, events, confirm, nil)
}
func buildDependenciesWithApprovals(cfg config.Config, events workflow.EventSink, confirm setup.Confirmation, workspaceApprover workflow.WorkspaceApprover) (workflow.Dependencies, error) {
	runner := process.NewOSRunner()
	agents, _ := buildAgentRunners(cfg, events, runner, confirm)
	workspace, err := folderworkspace.NewWorkspaceWithApproval(cfg.Workspace.Adapter(), workspaceApprover)
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

// Process decoration is shared by roles. Provider selection is fixed here at
// startup; the core sees only Planner, Implementer and Reviewer operations.
type agentRunners struct {
	schema, session, claude setup.Runner
}

func buildAgentRunners(cfg config.Config, events workflow.EventSink, runner process.OSRunner, confirm setup.Confirmation) (agentRunners, *setup.Manager) {
	if cfg.InstallMode != "prompt" {
		confirm = nil
	}
	installation := setup.NewManager(runner, confirm, time.Duration(cfg.InstallTimeout))
	var reportActivity func(activity.Event)
	if reporter, ok := events.(interface{ AgentActivity(activity.Event) }); ok {
		reportActivity = reporter.AgentActivity
	}
	var reportRuntime func(string) error
	if reporter, ok := events.(interface{ CodexRuntimeSelected(string) error }); ok {
		reportRuntime = reporter.CodexRuntimeSelected
	}
	return agentRunners{
		claude: setup.Runner{Runner: activity.Runner{Runner: runner, Agent: activity.Claude, Observe: reportActivity}, Tool: "claude", Manager: installation},
		session: setup.Runner{
			Runner:  activity.Runner{Runner: runner, Agent: activity.OpenCode, Observe: reportActivity},
			Tool:    "opencode",
			Manager: installation,
		},
		schema: setup.Runner{
			Runner:  schemaexec.NewRuntimeRunner(activity.Runner{Runner: runner, Agent: activity.Codex, Observe: reportActivity}, reportRuntime),
			Tool:    "codex",
			Manager: installation,
		},
	}, installation
}

func (r agentRunners) composePlanning(cfg config.Config, deps *workflow.Dependencies) error {
	planner, err := r.planner(cfg.Planner)
	if err != nil {
		return err
	}
	deps.Planner = planner
	if cfg.Fallback.Mode == "disabled" || cfg.Planner.Harness == "claude" {
		return nil
	}
	alternate, err := r.planner(cfg.Fallback.Planner)
	if err != nil {
		return err
	}
	deps.Planner, deps.Fallbacks.Planner = planner, alternate
	name := func(harness string) string {
		if harness == "opencode" {
			return "OpenCode"
		}
		return "Codex"
	}
	deps.Fallbacks.Planning = store.AgentSwitch{
		Stage: store.WorkflowStagePlanning,
		From:  name(cfg.Planner.Harness), To: name(cfg.Fallback.Planner.Harness),
		Model: modelName(cfg.Fallback.Planner.Model),
	}
	return nil
}

func (r agentRunners) planner(cfg config.Planner) (workflow.Planner, error) {
	switch cfg.Harness {
	case "claude":
		return schemaexec.NewClaude(r.claude, cfg.ClaudeAdapter())
	case "codex":
		return schemaexec.NewPlanner(r.schema, cfg.CodexAdapter())
	case "opencode":
		return sessionexec.NewReadOnlyAgent(r.session, cfg.OpenCodeAdapter())
	default:
		return nil, fmt.Errorf("planner.harness must be codex, opencode or claude")
	}
}

func (r agentRunners) composeImplementation(cfg config.Config, deps *workflow.Dependencies) error {
	if cfg.Implementer.Harness == "claude" {
		agent, err := schemaexec.NewClaude(r.claude, cfg.Implementer.ClaudeAdapter())
		deps.Implementer = agent
		return err
	}
	if cfg.Implementer.Harness == "codex" {
		implementer, err := schemaexec.NewImplementer(r.schema, cfg.Implementer.CodexAdapter())
		deps.Implementer = implementer
		// The existing billing route is OpenCode -> Codex. A primary Codex
		// implementer must not fall back to itself or request an unused login.
		return err
	}
	if cfg.Implementer.Harness != "opencode" {
		return fmt.Errorf("implementer.harness must be codex, opencode or claude")
	}
	implementer, err := sessionexec.NewImplementer(r.session, cfg.Implementer.OpenCodeAdapter())
	if err != nil {
		return err
	}
	deps.Implementer = implementer
	if cfg.Fallback.Mode == "disabled" {
		return nil
	}
	alternate, err := schemaexec.NewImplementer(r.schema, cfg.Fallback.CodexImplementer.Adapter())
	if err != nil {
		return err
	}
	deps.Implementer, deps.Fallbacks.Implementer = implementer, alternate
	deps.Fallbacks.Implementation = store.AgentSwitch{
		Stage:    store.WorkflowStageImplementation,
		From:     "OpenCode",
		To:       "Codex",
		Model:    cfg.Fallback.CodexImplementer.Model,
		CanWrite: true,
	}
	return nil
}

func (r agentRunners) composeReview(cfg config.Config, deps *workflow.Dependencies) error {
	switch cfg.Reviewer.Harness {
	case "claude":
		agent, err := schemaexec.NewClaude(r.claude, cfg.Reviewer.ClaudeAdapter())
		deps.Reviewer = agent
		return err
	case "opencode":
		agent, err := sessionexec.NewReadOnlyAgent(r.session, cfg.Reviewer.OpenCodeAdapter())
		deps.Reviewer = agent
		return err
	case "codex":
		reviewer, err := schemaexec.NewReviewer(r.schema, cfg.Reviewer.CodexAdapter())
		if err != nil {
			return err
		}
		deps.Reviewer = reviewer
		if cfg.Fallback.Mode == "disabled" {
			return nil
		}
		alternate, err := sessionexec.NewReadOnlyAgent(r.session, cfg.Fallback.OpenCodeReviewer.Adapter())
		if err != nil {
			return err
		}
		deps.Fallbacks.Reviewer = alternate
		deps.Fallbacks.Review = store.AgentSwitch{Stage: store.WorkflowStageReview, From: "Codex", To: "OpenCode", Model: modelName(cfg.Fallback.OpenCodeReviewer.Model)}
		return nil
	default:
		return fmt.Errorf("reviewer.harness must be codex, opencode or claude")
	}
}

func modelName(model string) string {
	if model == "" {
		return "CLI default"
	}
	return model
}
