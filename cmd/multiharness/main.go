// Command multiharness composes the plain-Go workflow with local CLI adapters.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"multiharness-core/internal/adapter/agent/activity"
	"multiharness-core/internal/adapter/agent/codex"
	"multiharness-core/internal/adapter/agent/opencode"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/adapter/setup"
	validationadapter "multiharness-core/internal/adapter/validation"
	gitworkspace "multiharness-core/internal/adapter/workspace/git"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	baseDir, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cannot determine invocation directory:", err)
		return cli.ExitUsage
	}
	approver := cli.NewTerminalApprover(os.Stdin, stderr)
	installer := cli.NewTerminalInstaller(os.Stdin, stderr)
	factory := func(cfg config.Config, events workflow.EventSink) (cli.Runner, error) {
		dependencies, err := buildDependenciesWithInstallation(cfg, events, cli.WithProgressInstallation(installer, events))
		if err != nil {
			return nil, err
		}
		if cfg.Fallback.Mode == "prompt" {
			dependencies.Fallbacks.Approver = cli.WithProgressApproval(approver, events)
		}
		return workflow.NewService(dependencies)
	}
	handler, err := cli.NewHandler(factory, stdout, stderr, baseDir, os.LookupEnv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return cli.ExitFailed
	}
	return handler.Run(ctx, args)
}

func buildWorkflow(cfg config.Config, events workflow.EventSink) (cli.Runner, error) {
	return buildWorkflowWithApproval(cfg, events, nil)
}

func buildWorkflowWithApproval(cfg config.Config, events workflow.EventSink, approver workflow.BillingApprover) (cli.Runner, error) {
	dependencies, err := buildDependencies(cfg, events)
	if err != nil {
		return nil, err
	}
	if cfg.Fallback.Mode == "prompt" {
		dependencies.Fallbacks.Approver = cli.WithProgressApproval(approver, events)
	}
	return workflow.NewService(dependencies)
}

// The same composition is used by production and opt-in integration tests.
// Tests may decorate a port to inject a reproducible fault, never agent output.
func buildDependencies(cfg config.Config, events workflow.EventSink) (workflow.Dependencies, error) {
	return buildDependenciesWithInstallation(cfg, events, nil)
}

func buildDependenciesWithInstallation(cfg config.Config, events workflow.EventSink, confirm setup.Confirmation) (workflow.Dependencies, error) {
	runner := process.NewOSRunner()
	if cfg.InstallMode != "prompt" {
		confirm = nil
	}
	installation := setup.NewManager(runner, confirm, time.Duration(cfg.InstallTimeout))
	var reportActivity func(activity.Event)
	if reporter, ok := events.(interface{ AgentActivity(activity.Event) }); ok {
		reportActivity = reporter.AgentActivity
	}
	openCodeRunner := setup.Runner{Runner: activity.Runner{Runner: runner, Agent: activity.OpenCode, Observe: reportActivity}, Tool: "opencode", Manager: installation}
	// Runtime notices belong to presentation, not the workflow state machine.
	var reportRuntime func(string) error
	if reporter, ok := events.(interface{ CodexRuntimeSelected(string) error }); ok {
		reportRuntime = reporter.CodexRuntimeSelected
	}
	codexRunner := setup.Runner{Runner: codex.NewRuntimeRunner(activity.Runner{Runner: runner, Agent: activity.Codex, Observe: reportActivity}, reportRuntime), Tool: "codex", Manager: installation}
	workspace, err := gitworkspace.NewWorkspace(setup.Runner{Runner: runner, Tool: "git", Manager: installation}, cfg.Git.Adapter())
	if err != nil {
		return workflow.Dependencies{}, err
	}
	planner, err := codex.NewPlanner(codexRunner, cfg.Planner.Adapter())
	if err != nil {
		return workflow.Dependencies{}, err
	}
	implementer, err := opencode.NewImplementer(openCodeRunner, cfg.Implementer.Adapter(), nil)
	if err != nil {
		return workflow.Dependencies{}, err
	}
	validator, err := validationadapter.NewValidator(runner, cfg.Validation.Adapter())
	if err != nil {
		return workflow.Dependencies{}, err
	}
	reviewer, err := codex.NewReviewer(codexRunner, cfg.Reviewer.Adapter())
	if err != nil {
		return workflow.Dependencies{}, err
	}
	alternateImplementation, err := codex.NewImplementer(codexRunner, cfg.Fallback.CodexImplementer.Adapter())
	if err != nil {
		return workflow.Dependencies{}, err
	}
	alternatePlanner, err := opencode.NewReadOnlyAgent(openCodeRunner, cfg.Fallback.OpenCodePlanner.Adapter())
	if err != nil {
		return workflow.Dependencies{}, err
	}
	alternateReviewer, err := opencode.NewReadOnlyAgent(openCodeRunner, cfg.Fallback.OpenCodeReviewer.Adapter())
	if err != nil {
		return workflow.Dependencies{}, err
	}
	modelName := func(model string) string {
		if model == "" {
			return "CLI default"
		}
		return model
	}
	fallbacks := workflow.BillingFallbacks{Planner: alternatePlanner, Implementer: alternateImplementation, Reviewer: alternateReviewer,
		Planning:       store.AgentSwitch{Stage: store.WorkflowStagePlanning, From: "Codex", To: "OpenCode", Model: modelName(cfg.Fallback.OpenCodePlanner.Model)},
		Review:         store.AgentSwitch{Stage: store.WorkflowStageReview, From: "Codex", To: "OpenCode", Model: modelName(cfg.Fallback.OpenCodeReviewer.Model)},
		Implementation: store.AgentSwitch{Stage: store.WorkflowStageImplementation, From: "OpenCode", To: "Codex", Model: cfg.Fallback.CodexImplementer.Model, CanWrite: true},
	}
	return workflow.Dependencies{Workspace: workspace, Planner: planner, Implementer: implementer, Validator: validator, Reviewer: reviewer, Events: events, Execution: cfg.Execution.Policy(), Fallbacks: fallbacks}, nil
}
