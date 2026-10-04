package main

import (
	"reflect"
	"testing"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/config"
	"multiharness-core/internal/workflow"
)

// Every harness must enforce the configured prompt budget and support chunked
// review, so mixing cheap implementers with stronger reviewers behaves the same
// whichever CLI serves each role.
func TestEveryHarnessReceivesBudgetAndBatchReview(t *testing.T) {
	want := structured.Budget{MaxPromptBytes: 65536, ReviewChunkBytes: 16384}
	for _, harness := range []string{"codex", "opencode", "claude", "muse"} {
		t.Run(harness, func(t *testing.T) {
			cfg, _ := fixtureConfiguration(t)
			helper := cfg.Planner.Executable
			cfg.Fallback.Mode = "disabled"
			cfg.Planner = config.DefaultPlanner(harness)
			cfg.Planner.Executable = helper
			cfg.Implementer = config.DefaultImplementer(harness)
			cfg.Implementer.Executable = helper
			cfg.Reviewer = config.DefaultPlanner(harness)
			cfg.Reviewer.Executable = helper
			cfg.Execution.MaxPromptBytes = want.MaxPromptBytes
			cfg.Execution.ReviewChunkBytes = want.ReviewChunkBytes
			deps, err := buildDependencies(cfg, nil)
			if err != nil {
				t.Fatal(err)
			}
			for role, agent := range map[string]any{"planner": deps.Planner, "implementer": deps.Implementer, "reviewer": deps.Reviewer} {
				field := reflect.ValueOf(agent).Elem().FieldByName("Budget")
				if !field.IsValid() || field.Interface() != want {
					t.Fatalf("%s budget = %v, want %v", role, field, want)
				}
			}
			if _, ok := deps.Reviewer.(workflow.BatchReviewer); !ok {
				t.Fatalf("%T does not support chunked review", deps.Reviewer)
			}
		})
	}
}
