package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

func TestAllCodexSetupIgnoresUnusedFallbacksIncludingSavedPromptMode(t *testing.T) {
	for _, mode := range []string{"disabled", "prompt"} {
		t.Run(mode, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.Mode = "team"
			cfg.Fallback.Mode = mode
			cfg.Implementer = config.DefaultImplementer("codex")
			cfg.Decision.Enabled = true
			var out bytes.Buffer
			h := &Handler{stdout: &out}
			calls, jev := 0, 0
			h.SetReadiness(func(ctx context.Context, r account.Request) account.Status {
				if r.Harness != "codex" {
					t.Fatalf("unused fallback checked: %+v", r)
				}
				calls++
				return account.Status{Ready: true}
			}, func(context.Context, config.Config, bool) account.Status { jev++; return account.Status{Ready: true} })
			// No sign-in answers are supplied: unused fallback accounts must never prompt.
			if err := h.completeAccountSetup(t.Context(), &setupLines{}, cfg, &interactiveView{writer: &out}); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || jev != 1 || strings.Contains(out.String(), "NEEDS SETUP") || strings.Contains(out.String(), "fallback planner") || !strings.Contains(out.String(), "Setup checks passed") {
				t.Fatal(calls, jev, out.String())
			}
		})
	}
}

func TestFallbackAccountCheckedOnlyAfterConsent(t *testing.T) {
	for _, stage := range []store.WorkflowStage{store.WorkflowStageAnswering, store.WorkflowStagePlanning, store.WorkflowStageReview, store.WorkflowStageImplementation, store.WorkflowStageRepair} {
		for _, consent := range []bool{false, true} {
			for _, ready := range []bool{false, true} {
				cfg := config.Defaults()
				cfg.Mode = "team"
				cfg.Fallback.Mode = "prompt"
				cfg.Fallback.Planner.Model = "provider/planner"
				cfg.Fallback.OpenCodeReviewer.Model = "provider/reviewer"
				target, model, canWrite := "OpenCode", cfg.Fallback.Planner.Model, false
				if stage == store.WorkflowStageReview {
					model = cfg.Fallback.OpenCodeReviewer.Model
				}
				if stage == store.WorkflowStageImplementation || stage == store.WorkflowStageRepair {
					target = "Codex"
					model = cfg.Fallback.CodexImplementer.Model
					canWrite = true
				}
				choice := store.AgentSwitch{Stage: stage, To: target, Model: model, CanWrite: canWrite}
				var out bytes.Buffer
				asked, checked := false, false
				wrapper := FallbackReadiness{Config: cfg, Output: &out, Approver: approvalFunc(func(context.Context, store.AgentSwitch) (bool, error) { asked = true; return consent, nil }), Check: func(ctx context.Context, r account.Request) account.Status {
					if !asked || !consent {
						t.Fatal("account checked before consent")
					}
					checked = true
					if r.Model != model || harnessName(r.Harness) != target {
						t.Fatal("wrong fallback checked", r)
					}
					return account.Status{Ready: ready, Detail: "Account is not signed in"}
				}}
				yes, err := wrapper.ConfirmFallback(t.Context(), choice)
				if err != nil || yes != (consent && ready) || checked != consent {
					t.Fatal(stage, consent, ready, yes, err)
				}
				if consent && !ready && !strings.Contains(out.String(), "Optional fallback was not started") {
					t.Fatal(out.String())
				}
			}
		}
	}
}
