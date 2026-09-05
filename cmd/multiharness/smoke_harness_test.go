package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

// Check the smoke harness's invocation limits and consent without models.
// The live tests remain responsible for actual CLI compatibility evidence.
func TestSmokeConfigurationBounds(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		env := map[string]string{}
		if explicit {
			env = map[string]string{
				"MULTIHARNESS_SMOKE_MODEL":          "primary/model",
				"MULTIHARNESS_SMOKE_FALLBACK_MODEL": "alternate/model",
				"MULTIHARNESS_SMOKE_STAGE_TIMEOUT":  "12s",
			}
		}
		cfg, err := config.Load("", t.TempDir(), nil, smokeOverrides(func(name string) string { return env[name] }))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Execution.MaxRetries != 0 || cfg.Execution.MaxAgentInvocations != 8 || cfg.Fallback.Mode != "disabled" || cfg.Timeout != config.Duration(20*time.Minute) {
			t.Fatal("smoke execution limits not enforced")
		}
		wantTimeout := config.Duration(5 * time.Minute)
		if explicit {
			wantTimeout = config.Duration(12 * time.Second)
			if cfg.Implementer.Model != "primary/model" || cfg.Fallback.OpenCodePlanner.Model != "alternate/model" || cfg.Fallback.OpenCodeReviewer.Model != "alternate/model" {
				t.Fatal("smoke model selection lost")
			}
		} else if cfg.Implementer.Model != "" || cfg.Fallback.OpenCodePlanner.Model != "" || cfg.Fallback.OpenCodeReviewer.Model != "" {
			t.Fatal("smoke silently selected a provider model")
		}
		for _, timeout := range []config.Duration{
			cfg.Planner.Timeout,
			cfg.Reviewer.Timeout,
			cfg.Implementer.Timeout,
			cfg.Fallback.CodexImplementer.Timeout,
			cfg.Fallback.OpenCodePlanner.Timeout,
			cfg.Fallback.OpenCodeReviewer.Timeout,
		} {
			if timeout != wantTimeout {
				t.Fatal("alternate or primary timeout did not apply")
			}
		}
		for _, stage := range smokeFallbackStages {
			_, err := smokeFallbackExecutable(cfg, stage)
			needsModel := stage == store.WorkflowStagePlanning || stage == store.WorkflowStageReview
			if (err != nil) != (needsModel && !explicit) {
				t.Fatal("alternate model selection gate mismatch")
			}
		}
		if _, err := smokeFallbackExecutable(cfg, store.WorkflowStageIntake); err == nil {
			t.Fatal("accepted unsupported fallback stage")
		}
	}
}

func TestSmokeConsentIsSingleUseAndRouteScoped(t *testing.T) {
	choice := store.AgentSwitch{Stage: store.WorkflowStagePlanning, From: "Codex", To: "OpenCode", Model: "fixture/model"}
	consent := &smokeConsent{expected: choice}
	if yes, err := consent.ConfirmFallback(t.Context(), choice); !yes || err != nil {
		t.Fatal("expected one scoped confirmation")
	}
	if yes, err := consent.ConfirmFallback(t.Context(), choice); yes || err == nil {
		t.Fatal("confirmed twice")
	}
	consent = &smokeConsent{expected: choice}
	choice.Stage = store.WorkflowStageReview
	if yes, err := consent.ConfirmFallback(t.Context(), choice); yes || err == nil {
		t.Fatal("confirmed unexpected role")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	consent = &smokeConsent{expected: choice}
	if yes, err := consent.ConfirmFallback(ctx, choice); yes || !errors.Is(err, context.Canceled) {
		t.Fatal("confirmed after cancellation")
	}
}
