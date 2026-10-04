package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/config"
	"multiharness-core/internal/contract"
	"multiharness-core/internal/transport/cli/screen"
)

type fallbackApprover func(context.Context, contract.AgentSwitch) (bool, error)

func (f fallbackApprover) ConfirmFallback(ctx context.Context, choice contract.AgentSwitch) (bool, error) {
	return f(ctx, choice)
}

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
			if err := h.completeAccountSetup(t.Context(), &setupLines{}, cfg, &screen.View{Writer: &out}); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || jev != 1 || strings.Contains(out.String(), "NEEDS SETUP") || strings.Contains(out.String(), "fallback planner") || !strings.Contains(out.String(), "Setup checks passed") {
				t.Fatal(calls, jev, out.String())
			}
		})
	}
}

func TestFallbackAccountCheckedOnlyAfterConsent(t *testing.T) {
	for _, stage := range []contract.WorkflowStage{contract.WorkflowStageAnswering, contract.WorkflowStagePlanning, contract.WorkflowStageReview, contract.WorkflowStageImplementation, contract.WorkflowStageRepair} {
		for _, consent := range []bool{false, true} {
			for _, ready := range []bool{false, true} {
				cfg := config.Defaults()
				cfg.Mode = "team"
				cfg.Fallback.Mode = "prompt"
				cfg.Fallback.Planner.Model = "provider/planner"
				cfg.Fallback.OpenCodeReviewer.Model = "provider/reviewer"
				target, model, canWrite := "OpenCode", cfg.Fallback.Planner.Model, false
				if stage == contract.WorkflowStageReview {
					model = cfg.Fallback.OpenCodeReviewer.Model
				}
				if stage == contract.WorkflowStageImplementation || stage == contract.WorkflowStageRepair {
					target = "Codex"
					model = cfg.Fallback.CodexImplementer.Model
					canWrite = true
				}
				choice := contract.AgentSwitch{Stage: stage, To: target, Model: model, CanWrite: canWrite}
				var out bytes.Buffer
				asked, checked := false, false
				wrapper := FallbackReadiness{Config: cfg, Output: &out, Approver: fallbackApprover(func(context.Context, contract.AgentSwitch) (bool, error) { asked = true; return consent, nil }), Check: func(ctx context.Context, r account.Request) account.Status {
					if !asked || !consent {
						t.Fatal("account checked before consent")
					}
					checked = true
					if r.Model != model || screen.HarnessName(r.Harness) != target {
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
