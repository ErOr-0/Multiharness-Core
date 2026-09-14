package main

import (
	"fmt"

	"multiharness-core/internal/adapter/agent/directexec"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/config"
	"multiharness-core/internal/delegation"
	"multiharness-core/internal/workflow"
)

func buildDelegation(cfg config.Config, events workflow.EventSink, confirm setup.Confirmation) (*delegation.Service, error) {
	if cfg.Workspace.ExistingWork != "snapshot" {
		return nil, fmt.Errorf("existing-work %s requires --mode team; direct mode uses the native CLI's workspace policy", cfg.Workspace.ExistingWork)
	}
	runners, _ := buildAgentRunners(cfg, events, process.NewOSRunner(), confirm)
	selected := cfg.Implementer
	var runner directexec.Runner
	switch selected.Harness {
	case "codex":
		runner = runners.schema
	case "claude":
		runner = runners.claude
	case "opencode":
		runner = runners.session
	}
	agent, err := directexec.New(runner, directexec.Config{
		Harness: selected.Harness, Executable: selected.Executable, Model: selected.Model,
		Reasoning: selected.Reasoning, Variant: selected.Variant,
		PermissionPolicy: string(selected.PermissionPolicy), ExtraArgs: selected.ExtraArgs,
	})
	if err != nil {
		return nil, err
	}
	timeout, name := cfg.DirectTimeout()
	return delegation.NewService(agent, timeout, name)
}
