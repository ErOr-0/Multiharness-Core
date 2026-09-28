package main

import (
	"fmt"

	"multiharness-core/internal/adapter/agent/directexec"
	"multiharness-core/internal/adapter/agent/schemaexec"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/config"
	"multiharness-core/internal/delegation"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func buildDelegation(cfg config.Config, events workflow.EventSink, confirm setup.Confirmation, nativeApprovers ...store.NativeApprover) (*delegation.Service, error) {
	if cfg.Workspace.ExistingWork != "snapshot" {
		return nil, fmt.Errorf("existing-work %s requires --mode team; direct mode uses the native CLI's workspace policy", cfg.Workspace.ExistingWork)
	}
	runners, _ := buildAgentRunners(cfg, events, process.NewOSRunner(), confirm)
	if len(nativeApprovers) > 0 {
		runners.approver = nativeApprovers[0]
	}
	selected := cfg.Implementer
	if selected.Harness == "muse" {
		agent, err := schemaexec.NewMuse(runners.muse, runners.museConfig(selected.MuseAdapter()))
		if err != nil {
			return nil, err
		}
		timeout, name := cfg.DirectTimeout()
		return delegation.NewService(agent, timeout, name)
	}
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
		Approver: runners.approver,
		Harness:  selected.Harness, Executable: selected.Executable, Model: selected.Model,
		Reasoning: selected.Reasoning, Variant: selected.Variant,
		Sandbox:          string(selected.Sandbox),
		PermissionPolicy: string(selected.PermissionPolicy), ExtraArgs: selected.ExtraArgs,
	})
	if err != nil {
		return nil, err
	}
	timeout, name := cfg.DirectTimeout()
	return delegation.NewService(agent, timeout, name)
}
