package main

import (
	"fmt"
	"time"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/adapter/agent/schemaexec"
	"multiharness-core/internal/adapter/agent/sessionexec"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// Process decoration is shared by roles. Provider selection is fixed here at
// startup; the core sees only Planner, Implementer and Reviewer operations.
type agentRunners struct {
	schema, session setup.Runner
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
	schemaPlanner, err := schemaexec.NewPlanner(r.schema, cfg.Planner.Adapter())
	if err != nil {
		return err
	}
	switch cfg.PlannerHarness {
	case "codex":
		alternate, err := sessionexec.NewReadOnlyAgent(r.session, cfg.Fallback.OpenCodePlanner.Adapter())
		if err != nil {
			return err
		}
		deps.Planner, deps.Fallbacks.Planner = schemaPlanner, alternate
		deps.Fallbacks.Planning = store.AgentSwitch{
			Stage: store.WorkflowStagePlanning,
			From:  "Codex",
			To:    "OpenCode",
			Model: modelName(cfg.Fallback.OpenCodePlanner.Model),
		}
	case "opencode":
		planner, err := sessionexec.NewReadOnlyAgent(r.session, cfg.OpenCodePlanner.Adapter())
		if err != nil {
			return err
		}
		deps.Planner, deps.Fallbacks.Planner = planner, schemaPlanner
		deps.Fallbacks.Planning = store.AgentSwitch{Stage: store.WorkflowStagePlanning, From: "OpenCode", To: "Codex", Model: cfg.Planner.Model}
	default:
		return fmt.Errorf("planner_harness must be codex or opencode")
	}
	return nil
}

func (r agentRunners) composeImplementation(cfg config.Config, deps *workflow.Dependencies) error {
	implementer, err := sessionexec.NewImplementer(r.session, cfg.Implementer.Adapter())
	if err != nil {
		return err
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
	reviewer, err := schemaexec.NewReviewer(r.schema, cfg.Reviewer.Adapter())
	if err != nil {
		return err
	}
	alternate, err := sessionexec.NewReadOnlyAgent(r.session, cfg.Fallback.OpenCodeReviewer.Adapter())
	if err != nil {
		return err
	}
	deps.Reviewer, deps.Fallbacks.Reviewer = reviewer, alternate
	deps.Fallbacks.Review = store.AgentSwitch{Stage: store.WorkflowStageReview, From: "Codex", To: "OpenCode", Model: modelName(cfg.Fallback.OpenCodeReviewer.Model)}
	return nil
}

func modelName(model string) string {
	if model == "" {
		return "CLI default"
	}
	return model
}
