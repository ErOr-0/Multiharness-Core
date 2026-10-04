package main

import (
	"fmt"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/adapter/agent/schemaexec"
	"multiharness-core/internal/adapter/agent/sessionexec"
	"multiharness-core/internal/adapter/agent/structured"
	decisionadapter "multiharness-core/internal/adapter/decision/openrouter"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/adapter/setup"
	validationadapter "multiharness-core/internal/adapter/validation"
	folderworkspace "multiharness-core/internal/adapter/workspace/folder"
	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

// The same composition is used by production and opt-in integration tests.
// Tests may decorate a port to inject a reproducible fault, never agent output.
func composeDependencies(cfg config.Config, events workflow.EventSink, confirm setup.Confirmation, workspaceApprover workflow.WorkspaceApprover, apiKey string, nativeApprovers ...contract.NativeApprover) (workflow.Dependencies, error) {
	runner := process.NewOSRunner()
	agents, _ := buildAgentRunners(cfg, events, runner, confirm)
	if len(nativeApprovers) > 0 {
		agents.approver = nativeApprovers[0]
	}
	workspaceConfig := cfg.Workspace.Adapter()
	if reporter, ok := events.(interface{ WorkspaceInspection(string, int) }); ok {
		workspaceConfig.Observe = func(p folderworkspace.ScanProgress) { reporter.WorkspaceInspection(p.Phase, p.Files) }
	}
	workspace, err := folderworkspace.NewWorkspaceWithApproval(workspaceConfig, workspaceApprover)
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
	if err := composeDecision(cfg, &dependencies, apiKey); err != nil {
		return workflow.Dependencies{}, err
	}
	return dependencies, nil
}

// Process decoration is shared by roles. Provider selection is fixed here at
// startup; the core sees only Planner, Implementer and Reviewer operations.
type agentRunners struct {
	schema, session, claude, muse setup.Runner
	approver                      contract.NativeApprover
	budget                        structured.Budget
}

// Every harness receives the same prompt budget so handoffs fail or split
// identically whichever CLI serves a role.
func (r agentRunners) codexConfig(c schemaexec.Config) schemaexec.Config {
	c.Approver, c.Budget = r.approver, r.budget
	return c
}

func (r agentRunners) opencodeConfig(c sessionexec.Config) sessionexec.Config {
	c.Approver, c.Budget = r.approver, r.budget
	return c
}
func (r agentRunners) claudeConfig(c schemaexec.ClaudeConfig) schemaexec.ClaudeConfig {
	c.Approver, c.Budget = r.approver, r.budget
	return c
}
func (r agentRunners) museConfig(c schemaexec.MuseConfig) schemaexec.MuseConfig {
	c.Approver, c.Budget = r.approver, r.budget
	return c
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
		budget: structured.Budget{MaxPromptBytes: cfg.Execution.MaxPromptBytes, ReviewChunkBytes: cfg.Execution.ReviewChunkBytes},
		muse:   setup.Runner{Runner: activity.Runner{Runner: runner, Agent: activity.Muse, Observe: reportActivity}, Tool: "muse"},
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
	if cfg.Fallback.Mode == "disabled" || (cfg.Planner.Harness == "claude" || cfg.Planner.Harness == "muse") {
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
	deps.Fallbacks.Planning = contract.AgentSwitch{
		Stage: contract.WorkflowStagePlanning,
		From:  name(cfg.Planner.Harness), To: name(cfg.Fallback.Planner.Harness),
		Model: modelName(cfg.Fallback.Planner.Model),
	}
	return nil
}

func (r agentRunners) planner(cfg config.Planner) (workflow.Planner, error) {
	switch cfg.Harness {
	case "muse":
		return schemaexec.NewMuse(r.muse, r.museConfig(cfg.MuseAdapter()))
	case "claude":
		return schemaexec.NewClaude(r.claude, r.claudeConfig(cfg.ClaudeAdapter()))
	case "codex":
		return schemaexec.NewPlanner(r.schema, r.codexConfig(cfg.CodexAdapter()))
	case "opencode":
		return sessionexec.NewReadOnlyAgent(r.session, r.opencodeConfig(cfg.OpenCodeAdapter()))
	default:
		return nil, fmt.Errorf("planner.harness must be codex, opencode, claude or muse")
	}
}

func (r agentRunners) composeImplementation(cfg config.Config, deps *workflow.Dependencies) error {
	if cfg.Implementer.Harness == "muse" {
		agent, err := schemaexec.NewMuse(r.muse, r.museConfig(cfg.Implementer.MuseAdapter()))
		deps.Implementer = agent
		return err
	}
	if cfg.Implementer.Harness == "claude" {
		agent, err := schemaexec.NewClaude(r.claude, r.claudeConfig(cfg.Implementer.ClaudeAdapter()))
		deps.Implementer = agent
		return err
	}
	if cfg.Implementer.Harness == "codex" {
		implementer, err := schemaexec.NewImplementer(r.schema, r.codexConfig(cfg.Implementer.CodexAdapter()))
		deps.Implementer = implementer
		// The existing billing route is OpenCode -> Codex. A primary Codex
		// implementer must not fall back to itself or request an unused login.
		return err
	}
	if cfg.Implementer.Harness != "opencode" {
		return fmt.Errorf("implementer.harness must be codex, opencode, claude or muse")
	}
	implementer, err := sessionexec.NewImplementer(r.session, r.opencodeConfig(cfg.Implementer.OpenCodeAdapter()))
	if err != nil {
		return err
	}
	deps.Implementer = implementer
	if cfg.Fallback.Mode == "disabled" {
		return nil
	}
	alternate, err := schemaexec.NewImplementer(r.schema, r.codexConfig(cfg.Fallback.CodexImplementer.Adapter()))
	if err != nil {
		return err
	}
	deps.Implementer, deps.Fallbacks.Implementer = implementer, alternate
	deps.Fallbacks.Implementation = contract.AgentSwitch{
		Stage:    contract.WorkflowStageImplementation,
		From:     "OpenCode",
		To:       "Codex",
		Model:    cfg.Fallback.CodexImplementer.Model,
		CanWrite: true,
	}
	return nil
}

func (r agentRunners) composeReview(cfg config.Config, deps *workflow.Dependencies) error {
	switch cfg.Reviewer.Harness {
	case "muse":
		agent, err := schemaexec.NewMuse(r.muse, r.museConfig(cfg.Reviewer.MuseAdapter()))
		deps.Reviewer = agent
		return err
	case "claude":
		agent, err := schemaexec.NewClaude(r.claude, r.claudeConfig(cfg.Reviewer.ClaudeAdapter()))
		deps.Reviewer = agent
		return err
	case "opencode":
		agent, err := sessionexec.NewReadOnlyAgent(r.session, r.opencodeConfig(cfg.Reviewer.OpenCodeAdapter()))
		deps.Reviewer = agent
		return err
	case "codex":
		reviewer, err := schemaexec.NewReviewer(r.schema, r.codexConfig(cfg.Reviewer.CodexAdapter()))
		if err != nil {
			return err
		}
		deps.Reviewer = reviewer
		if cfg.Fallback.Mode == "disabled" {
			return nil
		}
		fallback := cfg.Fallback.OpenCodeReviewer.Adapter()
		fallback.Budget = r.budget
		alternate, err := sessionexec.NewReadOnlyAgent(r.session, fallback)
		if err != nil {
			return err
		}
		deps.Fallbacks.Reviewer = alternate
		deps.Fallbacks.Review = contract.AgentSwitch{Stage: contract.WorkflowStageReview, From: "Codex", To: "OpenCode", Model: modelName(cfg.Fallback.OpenCodeReviewer.Model)}
		return nil
	default:
		return fmt.Errorf("reviewer.harness must be codex, opencode, claude or muse")
	}
}

func composeDecision(cfg config.Config, deps *workflow.Dependencies, apiKey string) error {
	if !cfg.Decision.Enabled {
		return nil
	}
	// Jev runs on the operator's own OpenRouter key; no provider token is
	// bundled or read from other variables.
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return fmt.Errorf("Jev is enabled but OPENROUTER_API_KEY is missing; enter it in an interactive terminal or configure the environment")
	}
	adapterCfg := cfg.Decision.Adapter(apiKey)
	client, err := decisionadapter.NewClient(adapterCfg)
	if err != nil {
		return err
	}
	deps.DecisionMaker = client
	return nil
}

func modelName(model string) string {
	if model == "" {
		return "CLI default"
	}
	return model
}
