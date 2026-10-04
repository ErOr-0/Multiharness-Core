package main

import (
	"encoding/json"
	"os"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
)

// Fake Muse runner rejects prompts above a small threshold; the complete
// planner -> Muse implementation -> review -> repair workflow must pass after
// the bounded-handoff fix.
func TestMuseHandoffBoundedPrompts(t *testing.T) {
	cfg, log := fixtureConfiguration(t)
	helper := cfg.Planner.Executable
	cfg.Fallback.Mode = "disabled"
	cfg.Planner = config.DefaultPlanner("muse")
	cfg.Planner.Executable = helper
	cfg.Planner.Model = "fixture-muse-plan"
	cfg.Planner.Reasoning = "low"
	cfg.Implementer = config.DefaultImplementer("muse")
	cfg.Implementer.Executable = helper
	cfg.Implementer.Model = "fixture-muse-implement"
	cfg.Implementer.Reasoning = "medium"
	cfg.Reviewer = config.DefaultPlanner("muse")
	cfg.Reviewer.Executable = helper
	cfg.Reviewer.Model = "fixture-muse-review"
	cfg.Reviewer.Reasoning = "high"
	// Small test threshold enforced by the fake runner and the handoff budget.
	cfg.Execution.MaxPromptBytes = 32768
	cfg.Execution.ReviewChunkBytes = 16384
	t.Setenv("MULTIHARNESS_FIXTURE_MAX_PROMPT_BYTES", "32768")
	svc, err := buildWorkflow(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	result := svc.Run(t.Context(), contract.TaskInput{Task: "fixture change", WorkingDir: cfg.WorkingDir, MaxRepairAttempts: 1})
	if result.Status != contract.TaskStatusApproved {
		data, _ := json.Marshal(result)
		t.Fatalf("bounded workflow failed: %s", data)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "muse-plan\nmuse-implement\ncheck\nmuse-review\nmuse-repair\ncheck\nmuse-review\n"
	if string(calls) != want {
		t.Fatalf("unexpected role handoff %q", calls)
	}
}
