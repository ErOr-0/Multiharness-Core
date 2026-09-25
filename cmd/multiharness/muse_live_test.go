package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

// Opt-in: uses the operator's Muse login and allowance, only in a disposable
// project. The deterministic check is executed by Multiharness, not the model.
func museLiveFixture(t *testing.T) (config.Config, store.TaskInput) {
	t.Helper()
	if os.Getenv("MULTIHARNESS_MUSE_LIVE") != "1" {
		t.Skip("set MULTIHARNESS_MUSE_LIVE=1 to use your Muse login")
	}
	dir := t.TempDir()
	for name, data := range map[string]string{
		"go.mod":      "module musefixture\n\ngo 1.26\n",
		"add.go":      "package fixture\n\nfunc Add(a, b int) int { return a - b }\n",
		"add_test.go": "package fixture\nimport \"testing\"\nfunc TestAdd(t *testing.T) { if Add(2,3)!=5 || Add(-4,3)!=-1 { t.Fatal(\"Add must add both integers\") } }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := config.Defaults()
	cfg.Mode, cfg.WorkingDir, cfg.Fallback.Mode = "team", dir, "disabled"
	cfg.Decision.Enabled = false
	cfg.Planner = config.DefaultPlanner("muse")
	cfg.Planner.Reasoning = "low"
	cfg.Implementer = config.DefaultImplementer("muse")
	cfg.Implementer.Reasoning = "medium"
	cfg.Reviewer = config.DefaultPlanner("muse")
	cfg.Reviewer.Reasoning = "high"
	cfg.Workspace.RecoveryDir = filepath.Join(t.TempDir(), "recovery")
	goexe, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Validation.Checks = []config.Check{{Executable: goexe, Args: []string{"test", "./..."}, Timeout: config.Duration(time.Minute)}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return cfg, store.TaskInput{Task: "Fix Add in add.go so it adds both integers. Preserve add_test.go and go.mod. This is an implementation request; produce a plan, implement the small fix, and review it against the existing test. File tools are available; Multiharness runs go test separately.", WorkingDir: dir, MaxRepairAttempts: 1}
}

func TestMuseLiveTeam(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows team workflows are unsupported; use Linux/WSL or Docker")
	}
	cfg, input := museLiveFixture(t)
	svc, err := buildWorkflow(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	result := svc.Run(ctx, input)
	if result.Status != store.TaskStatusApproved {
		data, _ := json.MarshalIndent(result, "", "  ")
		t.Fatalf("Muse team did not finish: %s", data)
	}
	t.Logf("Full team approved; calls=%d", result.AgentInvocations)
}

// Exercises real role handoff and deterministic validation even where the
// platform's full workspace/session lifecycle is not supported. This does not
// certify native Windows team orchestration.
func TestMuseLiveRoleHandoff(t *testing.T) {
	cfg, input := museLiveFixture(t)
	deps, err := buildDependencies(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()
	plan, err := deps.Planner.Plan(ctx, input)
	if err != nil {
		t.Fatal("planning:", err)
	}
	t.Log("Muse low: planning completed")
	implementation, err := deps.Implementer.Implement(ctx, store.ImplementationRequest{Input: input, Plan: plan})
	if err != nil {
		t.Fatal("implementation:", err)
	}
	t.Log("Muse medium: implementation completed")
	report, err := deps.Validator.Validate(ctx, store.ValidationRequest{Input: input, Plan: plan, Implementation: implementation})
	if err != nil || !report.Passed {
		t.Fatalf("validation: %+v %v", report, err)
	}
	t.Log("Independent go test passed")
	review, err := deps.Reviewer.Review(ctx, store.ReviewRequest{Input: input, Plan: plan, Implementation: implementation, Validation: report})
	if err != nil || !review.Approved {
		t.Fatalf("review: %+v %v", review, err)
	}
	t.Log("Muse high: review approved")
}
