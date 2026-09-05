package main

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

// The additional opt-in explicitly authorizes the harness's one scripted yes.
// Only billing failure and a source fault are injected. The selected alternate,
// implementation, validation and review must actually run and produce evidence.
// Never deliberately exhaust a real account to exercise a billing path.
func TestSmokeBillingFallback(t *testing.T) {
	if os.Getenv("MULTIHARNESS_SMOKE_FALLBACK") != "1" {
		t.Skip("additional opt-in: MULTIHARNESS_SMOKE_FALLBACK=1; see docs/testing.md")
	}
	base := smokeConfig(t, true)
	for _, stage := range smokeFallbackStages {
		t.Run(string(stage), func(t *testing.T) {
			cfg := base
			executable, err := smokeFallbackExecutable(cfg, stage)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := exec.LookPath(executable); err != nil {
				t.Fatal("selected fallback executable is unavailable")
			}
			cfg.WorkingDir = smokeRepository(t, cfg)
			runSmokeFallback(t, cfg, stage, buildDependencies)
		})
	}
}

var smokeFallbackStages = []store.WorkflowStage{store.WorkflowStagePlanning, store.WorkflowStageImplementation, store.WorkflowStageReview, store.WorkflowStageRepair}

func smokeFallbackExecutable(cfg config.Config, stage store.WorkflowStage) (string, error) {
	switch stage {
	case store.WorkflowStagePlanning, store.WorkflowStageReview:
		agent := cfg.Fallback.OpenCodePlanner
		if stage == store.WorkflowStageReview {
			agent = cfg.Fallback.OpenCodeReviewer
		}
		if agent.Model == "" {
			return "", errors.New("select the alternate OpenCode provider/model via MULTIHARNESS_SMOKE_FALLBACK_MODEL or the role's model in MULTIHARNESS_SMOKE_CONFIG")
		}
		return agent.Executable, nil
	case store.WorkflowStageImplementation, store.WorkflowStageRepair:
		return cfg.Fallback.CodexImplementer.Executable, nil
	default:
		return "", errors.New("unsupported smoke fallback stage")
	}
}

type smokeDependencyBuilder func(config.Config, workflow.EventSink) (workflow.Dependencies, error)

// The same harness also runs against deterministic ports, testing the test's
// fault-injection, partial-evidence, consent and session assertions offline.
func runSmokeFallback(t *testing.T, cfg config.Config, stage store.WorkflowStage, build smokeDependencyBuilder) {
	t.Helper()
	cfg.Fallback.Mode = "prompt"
	cfg.MaxRepairAttempts = 1
	if stage == store.WorkflowStagePlanning {
		cfg.MaxRepairAttempts = 0
	}
	var fault *smokeBillingFault
	var alternate *smokeAlternateImplementer
	var primary *smokeRepairProbe
	var consent *smokeConsent
	factory := func(cfg config.Config, events workflow.EventSink) (cli.Runner, error) {
		deps, err := build(cfg, events)
		if err != nil {
			return nil, err
		}
		primary = &smokeRepairProbe{Implementer: deps.Implementer, inject: stage == store.WorkflowStageReview || stage == store.WorkflowStageRepair}
		deps.Implementer = primary
		fault = &smokeBillingFault{Implementer: primary}
		alternate = &smokeAlternateImplementer{Implementer: deps.Fallbacks.Implementer, inject: stage == store.WorkflowStageImplementation}
		deps.Fallbacks.Implementer = alternate
		var choice store.AgentSwitch
		switch stage {
		case store.WorkflowStagePlanning:
			deps.Planner, choice = fault, deps.Fallbacks.Planning
		case store.WorkflowStageReview:
			deps.Reviewer, choice = fault, deps.Fallbacks.Review
		default:
			fault.partial = stage == store.WorkflowStageImplementation
			deps.Implementer, choice = fault, deps.Fallbacks.Implementation
			choice.Stage = stage
		}
		consent = &smokeConsent{expected: choice}
		deps.Fallbacks.Approver = consent
		return workflow.NewService(deps)
	}
	result := runSmokeCLI(t, cfg, factory)
	if fault.calls != 1 || consent.calls != 1 || len(result.AgentSwitches) != 1 || result.AgentSwitches[0] != consent.expected {
		t.Fatal("did not exercise exactly one billing failure and matching consented switch")
	}
	wantInvocations := 6
	if stage == store.WorkflowStagePlanning {
		wantInvocations = 4
	}
	if result.RepairAttempts != cfg.MaxRepairAttempts || result.AgentInvocations != wantInvocations {
		t.Fatal("handoff repair/launch accounting mismatch")
	}
	if stage == store.WorkflowStageImplementation && (alternate.implementations != 1 || alternate.repairs != 1) {
		t.Fatal("implementation switch was not sticky through later repair")
	}
	if stage == store.WorkflowStageRepair && (alternate.implementations != 0 || alternate.repairs != 1 || primary.session == "") {
		t.Fatal("did not exercise a cross-provider repair handoff")
	}
	if stage == store.WorkflowStageReview && primary.repairs != 1 {
		t.Fatal("review switch did not complete a same-session OpenCode repair")
	}
	if stage == store.WorkflowStagePlanning || stage == store.WorkflowStageReview {
		if alternate.implementations != 0 || alternate.repairs != 0 {
			t.Fatal("read-only switch changed the implementation provider")
		}
	}
	t.Logf(
		"approved; injected billing at %s; repairs=%d; launches=%d; independent evidence; run=%s",
		stage,
		result.RepairAttempts,
		result.AgentInvocations,
		result.RunID,
	)
}

// Consent is scoped to this single expected route. A second billing failure,
// a different role, or an unexpected destination can never receive another yes.
type smokeConsent struct {
	expected store.AgentSwitch
	calls    int
}

func (s *smokeConsent) ConfirmFallback(ctx context.Context, choice store.AgentSwitch) (bool, error) {
	s.calls++
	if s.calls != 1 || choice != s.expected {
		return false, errors.New("unexpected smoke billing consent request")
	}
	return (cli.BillingConfirmation{Input: smokeYes{}, Output: io.Discard}).ConfirmFallback(ctx, choice)
}

type smokeYes struct{}

func (smokeYes) ReadConfirmation(context.Context) (string, error) { return "yes", nil }

type smokeBillingFault struct {
	workflow.Implementer
	partial bool
	calls   int
}

func (f *smokeBillingFault) failure() error {
	f.calls++
	return &store.ProviderFailure{Kind: store.ProviderBillingExhausted, Attempts: 1}
}

func (f *smokeBillingFault) Plan(context.Context, store.TaskInput) (store.Plan, error) {
	return store.Plan{}, f.failure()
}

func (f *smokeBillingFault) Review(context.Context, store.ReviewRequest) (store.Review, error) {
	return store.Review{}, f.failure()
}

func (f *smokeBillingFault) Implement(ctx context.Context, request store.ImplementationRequest) (store.ImplementationResult, error) {
	if !f.partial {
		return f.Implementer.Implement(ctx, request)
	}
	if err := os.WriteFile(filepath.Join(request.Input.WorkingDir, "sum.go"), []byte(smokeFault), 0600); err != nil {
		return store.ImplementationResult{}, errors.New("inject partial smoke change")
	}
	return store.ImplementationResult{}, f.failure()
}

func (f *smokeBillingFault) ApplyReview(_ context.Context, request store.RepairRequest) (store.ImplementationResult, error) {
	if request.Validate() != nil || request.Implementation.AgentSessionID == "" || request.Validation.Passed {
		return store.ImplementationResult{}, errors.New("missing original session or failed evidence before repair handoff")
	}
	return store.ImplementationResult{}, f.failure()
}

type smokeAlternateImplementer struct {
	workflow.Implementer
	inject                   bool
	implementations, repairs int
}

func (p *smokeAlternateImplementer) Implement(ctx context.Context, request store.ImplementationRequest) (store.ImplementationResult, error) {
	p.implementations++
	if request.Validate() != nil || !smokePartialEvidence(request.Repository) {
		return store.ImplementationResult{}, errors.New("alternate did not receive original plan and partial-change evidence")
	}
	result, err := p.Implementer.Implement(ctx, request)
	if err != nil {
		return result, err
	}
	if result.AgentSessionID != "" {
		return result, errors.New("ephemeral Codex implementation returned a persistent session")
	}
	if p.inject {
		err = os.WriteFile(filepath.Join(request.Input.WorkingDir, "sum.go"), []byte(smokeFault), 0600)
	}
	return result, err
}

func (p *smokeAlternateImplementer) ApplyReview(ctx context.Context, request store.RepairRequest) (store.ImplementationResult, error) {
	p.repairs++
	if request.Validate() != nil || request.Implementation.AgentSessionID != "" || request.Validation.Passed || !smokePartialEvidence(request.Repository) {
		return store.ImplementationResult{}, errors.New("alternate repair lost feedback/evidence or received a cross-provider session")
	}
	result, err := p.Implementer.ApplyReview(ctx, request)
	if err == nil && result.AgentSessionID != "" {
		return result, errors.New("ephemeral Codex repair returned a persistent session")
	}
	return result, err
}

func smokePartialEvidence(evidence *store.RepositoryEvidence) bool {
	return evidence != nil && evidence.Complete && evidence.Baseline.Fingerprint != evidence.Current.Fingerprint && slices.Equal(evidence.ChangedFiles, []string{"sum.go"}) && len(evidence.PreservationViolations) == 0
}
