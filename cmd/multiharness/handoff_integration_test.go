package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func fixtureHandoff(prompt []byte) error {
	if os.Getenv("MULTIHARNESS_FIXTURE_HANDOFF") != "1" {
		return nil
	}
	for _, text := range []string{"Keep the public API unchanged", "Preserve the original result format", "fixture change with prior constraints"} {
		if !bytes.Contains(prompt, []byte(text)) {
			return errors.New("cross-provider handoff lost user context: " + text)
		}
	}
	if bytes.Contains(prompt, []byte("Implementation request:")) || bytes.Contains(prompt, []byte("Review request:")) || bytes.Contains(prompt, []byte("Repair request:")) {
		for _, text := range []string{"The fixture result check identifies the requested behavior", "result check passes", "baseline"} {
			if !bytes.Contains(prompt, []byte(text)) {
				return errors.New("cross-provider handoff lost plan or evidence: " + text)
			}
		}
	}
	return nil
}

type fixturePermissionResolver func(context.Context, store.WorkflowStage, store.PermissionDenied) (bool, error)

func (f fixturePermissionResolver) ResolvePermission(ctx context.Context, stage store.WorkflowStage, denied store.PermissionDenied) (bool, error) {
	return f(ctx, stage, denied)
}

func TestMixedProviderHandoffAndPermissionRecoveryIntegration(t *testing.T) {
	cfg, log := fixtureConfiguration(t)
	helper := cfg.Planner.Executable
	cfg.Fallback.Mode = "disabled"
	cfg.Planner = config.DefaultPlanner("muse")
	cfg.Planner.Executable, cfg.Planner.Model, cfg.Planner.Reasoning = helper, "fixture-muse-plan", "low"
	cfg.Implementer = config.DefaultImplementer("claude")
	cfg.Implementer.Executable, cfg.Implementer.Model, cfg.Implementer.Reasoning = helper, "fixture-claude-implement", "medium"
	t.Setenv("MULTIHARNESS_FIXTURE_HANDOFF", "1")
	flag := filepath.Join(t.TempDir(), "permission-resolved")
	t.Setenv("MULTIHARNESS_FIXTURE_PERMISSION_FLAG", flag)
	deps, err := buildDependencies(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	prompts := 0
	deps.PermissionResolver = fixturePermissionResolver(func(_ context.Context, stage store.WorkflowStage, denied store.PermissionDenied) (bool, error) {
		prompts++
		if stage != store.WorkflowStageImplementation || denied.Action.Tool != "Write" || denied.Action.Target != "result.txt" {
			t.Fatal(stage, denied)
		}
		return true, os.WriteFile(flag, []byte("resolved"), 0600)
	})
	svc, err := workflow.NewService(deps)
	if err != nil {
		t.Fatal(err)
	}
	result := svc.Run(t.Context(), store.TaskInput{Task: "fixture change with prior constraints", WorkingDir: cfg.WorkingDir, MaxRepairAttempts: 1, RecentTurns: []store.ConversationTurn{
		{User: "Keep the public API unchanged", Assistant: "Understood"},
		{User: "Preserve the original result format", Assistant: "Understood"},
	}})
	if result.Status != store.TaskStatusApproved || prompts != 1 || result.AgentInvocations != 6 {
		t.Fatalf("unexpected recovery: %+v; failure: %+v", result, result.Failure)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(calls) != "muse-plan\nclaude-blocked\nclaude-implement\ncheck\nreview\nclaude-repair\ncheck\nreview\n" {
		t.Fatal("completed stages replayed", string(calls))
	}
	partial, err := os.ReadFile(filepath.Join(cfg.WorkingDir, "partial.txt"))
	if err != nil || string(partial) != "keep partial work" {
		t.Fatal("partial work lost", err)
	}
	if result.Repository.Baseline.Fingerprint == result.Repository.Current.Fingerprint || !strings.Contains(result.Repository.Diff, "partial.txt") {
		t.Fatal("baseline-relative evidence lost", result.Repository)
	}
}
