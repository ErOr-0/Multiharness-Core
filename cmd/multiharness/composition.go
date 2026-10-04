package main

import (
	"fmt"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/adapter/agent/schemaexec"
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
	schema, claude, muse setup.Runner
	approver             contract.NativeApprover
	budget               structured.Budget
}

// Every harness receives the same prompt budget so handoffs fail or split
// identically whichever CLI serves a role.
func (r agentRunners) codexConfig(c schemaexec.Config) schemaexec.Config {
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
		schema: setup.Runner{
			Runner:  schemaexec.NewRuntimeRunner(activity.Runner{Runner: runner, Agent: activity.Codex, Observe: reportActivity}, reportRuntime),
			Tool:    "codex",
			Manager: installation,
		},
	}, installation
}

func (r agentRunners) composePlanning(cfg config.Config, deps *workflow.Dependencies) error {
	planner, err := r.planner(cfg.Planner)
	deps.Planner = planner
	return err
}

func (r agentRunners) planner(cfg config.Planner) (workflow.Planner, error) {
	switch cfg.Harness {
	case "muse":
		return schemaexec.NewMuse(r.muse, r.museConfig(cfg.MuseAdapter()))
	case "claude":
		return schemaexec.NewClaude(r.claude, r.claudeConfig(cfg.ClaudeAdapter()))
	case "codex":
		return schemaexec.NewPlanner(r.schema, r.codexConfig(cfg.CodexAdapter()))
	default:
		return nil, fmt.Errorf("planner.harness must be codex, claude or muse")
	}
}

func (r agentRunners) composeImplementation(cfg config.Config, deps *workflow.Dependencies) error {
	var err error
	switch cfg.Implementer.Harness {
	case "muse":
		deps.Implementer, err = schemaexec.NewMuse(r.muse, r.museConfig(cfg.Implementer.MuseAdapter()))
	case "claude":
		deps.Implementer, err = schemaexec.NewClaude(r.claude, r.claudeConfig(cfg.Implementer.ClaudeAdapter()))
	case "codex":
		deps.Implementer, err = schemaexec.NewImplementer(r.schema, r.codexConfig(cfg.Implementer.CodexAdapter()))
	default:
		err = fmt.Errorf("implementer.harness must be codex, claude or muse")
	}
	return err
}

func (r agentRunners) composeReview(cfg config.Config, deps *workflow.Dependencies) error {
	var err error
	switch cfg.Reviewer.Harness {
	case "muse":
		deps.Reviewer, err = schemaexec.NewMuse(r.muse, r.museConfig(cfg.Reviewer.MuseAdapter()))
	case "claude":
		deps.Reviewer, err = schemaexec.NewClaude(r.claude, r.claudeConfig(cfg.Reviewer.ClaudeAdapter()))
	case "codex":
		deps.Reviewer, err = schemaexec.NewReviewer(r.schema, r.codexConfig(cfg.Reviewer.CodexAdapter()))
	default:
		err = fmt.Errorf("reviewer.harness must be codex, claude or muse")
	}
	return err
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
