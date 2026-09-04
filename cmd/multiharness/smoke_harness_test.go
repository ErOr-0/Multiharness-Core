package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

// Check the smoke harness's deliberate fault/session assertions without models.
// The live tests remain responsible for actual CLI compatibility evidence.
type probeImplementer struct {
	err     error
	session string
}

func TestSmokeConfigurationBounds(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		env := map[string]string{}
		if explicit {
			env = map[string]string{"MULTIHARNESS_SMOKE_MODEL": "primary/model", "MULTIHARNESS_SMOKE_FALLBACK_MODEL": "alternate/model", "MULTIHARNESS_SMOKE_STAGE_TIMEOUT": "12s"}
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
		for _, timeout := range []config.Duration{cfg.Planner.Timeout, cfg.Reviewer.Timeout, cfg.Implementer.Timeout, cfg.Fallback.CodexImplementer.Timeout, cfg.Fallback.OpenCodePlanner.Timeout, cfg.Fallback.OpenCodeReviewer.Timeout} {
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

func TestSmokeFallbackHarnessOffline(t *testing.T) {
	for _, stage := range smokeFallbackStages {
		t.Run(string(stage), func(t *testing.T) {
			cfg := config.Defaults()
			cfg.WorkingDir = smokeRepository(t, cfg)
			cfg.Execution.MaxAgentInvocations = 8
			cfg.Fallback.OpenCodePlanner.Model = "fixture/model"
			cfg.Fallback.OpenCodeReviewer.Model = "fixture/model"
			build := func(cfg config.Config, events workflow.EventSink) (workflow.Dependencies, error) {
				deps, err := buildDependencies(cfg, events)
				if err != nil {
					return deps, err
				}
				// Constructors above never invoke agents. All model ports are
				// replaced; Git, Go validation, CLI and workflow stay real.
				agent := smokeOfflineAgent{session: "fixture-session"}
				deps.Planner, deps.Implementer, deps.Reviewer = agent, agent, agent
				deps.Fallbacks.Planner, deps.Fallbacks.Reviewer = agent, agent
				deps.Fallbacks.Implementer = smokeOfflineAgent{}
				return deps, nil
			}
			runSmokeFallback(t, cfg, stage, build)
		})
	}
}

type smokeOfflineAgent struct{ session string }

func (smokeOfflineAgent) Plan(context.Context, store.TaskInput) (store.Plan, error) {
	return store.Plan{Action: store.PlanActionImplement, Summary: "Fix addition", Steps: []string{"Return a+b in sum.go"}, AcceptanceCriteria: []string{"Addition tests pass"}}, nil
}

func (a smokeOfflineAgent) Implement(_ context.Context, request store.ImplementationRequest) (store.ImplementationResult, error) {
	err := os.WriteFile(filepath.Join(request.Input.WorkingDir, "sum.go"), []byte("package example\n\nfunc Add(a, b int) int { return a + b }\n"), 0600)
	return store.ImplementationResult{Summary: "Fixed addition", ChangedFiles: []string{"sum.go"}, AgentSessionID: a.session}, err
}

func (a smokeOfflineAgent) ApplyReview(ctx context.Context, request store.RepairRequest) (store.ImplementationResult, error) {
	return a.Implement(ctx, store.ImplementationRequest{Input: request.Input, Plan: request.Plan, Repository: request.Repository})
}

func (smokeOfflineAgent) Review(_ context.Context, request store.ReviewRequest) (store.Review, error) {
	review := store.Review{Approved: request.Validation.Passed, Summary: "Independent checks reviewed"}
	if !review.Approved {
		review.Findings = []store.ReviewFinding{{Severity: store.FindingSeverityError, Blocking: true, File: "sum.go", Description: "Addition is incorrect", Evidence: "Go tests failed", RequiredAction: "Return a+b"}}
	}
	return review, nil
}

func (p probeImplementer) Implement(context.Context, store.ImplementationRequest) (store.ImplementationResult, error) {
	return store.ImplementationResult{Summary: "implemented", AgentSessionID: p.session}, p.err
}
func (p probeImplementer) ApplyReview(context.Context, store.RepairRequest) (store.ImplementationResult, error) {
	return store.ImplementationResult{Summary: "repaired", AgentSessionID: p.session}, p.err
}

func TestSmokeHarnessFaultInjectionAndSessionChecks(t *testing.T) {
	repo := t.TempDir()
	source := filepath.Join(repo, "sum.go")
	request := store.ImplementationRequest{Input: store.TaskInput{WorkingDir: repo}}
	sentinel := errors.New("agent failed")
	for _, inject := range []bool{false, true} {
		for _, agentErr := range []error{nil, sentinel} {
			if err := os.WriteFile(source, []byte(smokeSource), 0600); err != nil {
				t.Fatal(err)
			}
			probe := &smokeRepairProbe{Implementer: probeImplementer{err: agentErr, session: "session"}, inject: inject}
			_, err := probe.Implement(t.Context(), request)
			if !errors.Is(err, agentErr) {
				t.Fatal("lost implementation error")
			}
			data, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			changed := string(data) != smokeSource
			if changed != (inject && agentErr == nil) {
				t.Fatal("injected outside the successful repair-test path")
			}
		}
	}
	feedback := store.RepairRequest{
		Implementation: store.ImplementationResult{AgentSessionID: "session"},
		Validation:     store.ValidationReport{Passed: false},
		Review:         store.Review{Approved: false, Findings: []store.ReviewFinding{{Blocking: true}}},
	}
	for _, scenario := range []string{"valid", "no failed checks", "no findings", "nonblocking findings", "approved", "wrong prior session", "wrong resumed session", "agent error"} {
		t.Run(scenario, func(t *testing.T) {
			probe := &smokeRepairProbe{Implementer: probeImplementer{session: "session"}, session: "session"}
			input := feedback
			switch scenario {
			case "no failed checks":
				input.Validation.Passed = true
			case "no findings":
				input.Review.Findings = nil
			case "nonblocking findings":
				input.Review.Findings = []store.ReviewFinding{{Blocking: false}}
			case "approved":
				input.Review.Approved = true
			case "wrong prior session":
				input.Implementation.AgentSessionID = "different"
			case "wrong resumed session":
				probe.Implementer = probeImplementer{session: "different"}
			case "agent error":
				probe.Implementer = probeImplementer{session: "session", err: sentinel}
			}
			_, err := probe.ApplyReview(t.Context(), input)
			if (err == nil) != (scenario == "valid") {
				t.Fatal("repair/session gate mismatch")
			}
			if scenario == "valid" && probe.repairs != 1 {
				t.Fatal("repair not counted")
			}
		})
	}
}
