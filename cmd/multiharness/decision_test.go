package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func TestDecisionCompositionRequiresExplicitKeyWhenEnabled(t *testing.T) {
	cfg := config.Defaults()
	deps := workflow.Dependencies{}
	if err := composeDecision(cfg, &deps, ""); err != nil || deps.DecisionMaker != nil {
		t.Fatal("disabled router must not require a key")
	}
	cfg.Decision.Enabled = true
	if err := composeDecision(cfg, &deps, ""); err == nil || !strings.Contains(err.Error(), "OPENROUTER_API_KEY") || deps.DecisionMaker != nil {
		t.Fatal("missing key silently enabled fallback routing")
	}
	if err := composeDecision(cfg, &deps, "test-key"); err != nil || deps.DecisionMaker == nil {
		t.Fatal("explicit session key did not compose router")
	}
}

func TestScriptedJevWithoutKeyStopsBeforeAgents(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("MULTIHARNESS_INSTALL_MODE", "disabled")
	var stdout, stderr bytes.Buffer
	code := run([]string{"--mode", "team", "--decision-enabled", "true", "--task", "Fix a typo"}, &stdout, &stderr)
	if code != cli.ExitUsage {
		t.Fatalf("exit=%d, want configuration error", code)
	}
	var result cli.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Failure == nil || !strings.Contains(result.Failure.Message, "OPENROUTER_API_KEY") {
		t.Fatal("missing actionable key setup instructions")
	}
	if result.Failure.Stage != store.WorkflowStageIntake || result.Plan != nil || result.Implementation != nil {
		t.Fatal("workflow ran without required credentials")
	}
}
