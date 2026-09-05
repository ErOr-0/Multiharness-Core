package workflow_test

import (
	"context"
	"errors"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type approvalFunc func(context.Context, store.AgentSwitch) (bool, error)

func (f approvalFunc) ConfirmFallback(ctx context.Context, r store.AgentSwitch) (bool, error) {
	return f(ctx, r)
}
func billingError() error {
	return &store.ProviderFailure{Kind: store.ProviderBillingExhausted, Attempts: 1}
}
func installFallback(t *testing.T, h *workflowHarness, approval workflow.BillingApprover, limit int) *fakeImplementer {
	t.Helper()
	alternate := &fakeImplementer{
		workspace: h.workspace,
		initial:   implementation("alternate", "service.go"),
		repairs:   []store.ImplementationResult{implementation("fixed", "service.go")},
	}
	f := workflow.BillingFallbacks{
		Planner:        &fakePlanner{plan: validPlan()},
		Implementer:    alternate,
		Reviewer:       &fakeReviewer{reviews: []store.Review{approvedReview("alternate approved")}},
		Approver:       approval,
		Planning:       store.AgentSwitch{Stage: store.WorkflowStagePlanning, From: "Primary", To: "Alternate", Model: "model"},
		Review:         store.AgentSwitch{Stage: store.WorkflowStageReview, From: "Primary", To: "Alternate", Model: "model"},
		Implementation: store.AgentSwitch{Stage: store.WorkflowStageImplementation, From: "Primary", To: "Alternate", Model: "model", CanWrite: true},
	}
	s, err := workflow.NewService(workflow.Dependencies{
		Workspace:   h.workspace,
		Planner:     h.planner,
		Implementer: h.implementer,
		Validator:   h.validator,
		Reviewer:    h.reviewer,
		Fallbacks:   f,
		Execution:   workflow.ExecutionPolicy{MaxAgentInvocations: limit},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.service = s
	return alternate
}

func TestBillingFallbackRequiresConsentAtEveryRole(t *testing.T) {
	for _, stage := range []store.WorkflowStage{store.WorkflowStagePlanning, store.WorkflowStageImplementation, store.WorkflowStageReview, store.WorkflowStageRepair} {
		for _, yes := range []bool{false, true} {
			t.Run(string(stage)+map[bool]string{true: "/yes", false: "/no"}[yes], func(t *testing.T) {
				h := newWorkflowHarness(t)
				prompts := 0
				alternate := installFallback(t, h, approvalFunc(func(_ context.Context, choice store.AgentSwitch) (bool, error) {
					prompts++
					if choice.Stage != stage {
						t.Fatal("wrong role")
					}
					return yes, nil
				}), 20)
				switch stage {
				case store.WorkflowStagePlanning:
					h.planner.err = billingError()
				case store.WorkflowStageImplementation:
					h.implementer.initialErr = billingError()
				case store.WorkflowStageReview:
					h.reviewer.err = billingError()
				case store.WorkflowStageRepair:
					h.reviewer.reviews = []store.Review{rejectedReview("repair"), approvedReview("done")}
					h.validator.reports = []store.ValidationReport{failingValidation(), passingValidation()}
					h.implementer.repairErr = billingError()
				}
				result := h.service.Run(t.Context(), validTask(2))
				if prompts != 1 {
					t.Fatalf("prompts=%d", prompts)
				}
				if yes && (result.Status != store.TaskStatusApproved || len(result.AgentSwitches) != 1) {
					t.Fatalf("switch failed: %+v", result.Failure)
				}
				if !yes && (result.Status != store.TaskStatusFailed || len(result.AgentSwitches) != 0) {
					t.Fatal("decline continued")
				}
				if stage == store.WorkflowStageRepair && yes && alternate.repairCalls[0].Implementation.AgentSessionID != "" {
					t.Fatal("cross-provider session leak")
				}
				if err := result.Validate(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestBillingHandoffRetainsPartialWorkAndStaysOnAlternateForRepairs(t *testing.T) {
	h := newWorkflowHarness(t)
	prompts := 0
	alternate := installFallback(t, h, approvalFunc(func(context.Context, store.AgentSwitch) (bool, error) { prompts++; return true, nil }), 20)
	h.implementer.implement = func(context.Context, store.ImplementationRequest) (store.ImplementationResult, error) {
		h.workspace.session.current.Current.Fingerprint = "partial"
		h.workspace.session.current.ChangedFiles = []string{"partial.go"}
		return store.ImplementationResult{}, billingError()
	}
	alternate.implement = func(_ context.Context, r store.ImplementationRequest) (store.ImplementationResult, error) {
		if r.Repository.Current.Fingerprint != "partial" || len(r.Plan.Steps) == 0 {
			t.Fatal("lost partial evidence or plan")
		}
		return implementation("continued", "service.go"), nil
	}
	h.reviewer.reviews = []store.Review{rejectedReview("repair"), approvedReview("fixed")}
	h.validator.reports = []store.ValidationReport{failingValidation(), passingValidation()}
	result := h.service.Run(t.Context(), validTask(1))
	if result.Status != store.TaskStatusApproved || prompts != 1 || len(alternate.repairCalls) != 1 || len(h.implementer.repairCalls) != 0 || result.AgentInvocations != 6 {
		t.Fatalf("bad sticky switch: %+v", result)
	}
}

func TestFallbackStopsForUnsafeConditions(t *testing.T) {
	for _, mode := range []string{
		"nil approver",
		"budget",
		"cancel",
		"prompt mutation",
		"read-only mutation",
		"alternate billing",
		"unknown error",
		"approval error",
	} {
		t.Run(mode, func(t *testing.T) {
			h := newWorkflowHarness(t)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			prompts := 0
			var approval workflow.BillingApprover = approvalFunc(func(context.Context, store.AgentSwitch) (bool, error) {
				prompts++
				switch mode {
				case "cancel":
					cancel()
				case "prompt mutation":
					h.workspace.session.current.Current.Fingerprint = "concurrent edit"
				case "approval error":
					return false, errors.New("input unavailable")
				}
				return true, nil
			})
			limit := 20
			if mode == "budget" {
				limit = 1
			}
			if mode == "nil approver" {
				approval = nil
			}
			alternate := installFallback(t, h, approval, limit)
			h.planner.err = billingError()
			if mode == "unknown error" {
				h.planner.err = errors.New("quota string alone must not trigger consent")
			}
			if mode == "read-only mutation" {
				h.planner.run = func(context.Context, store.TaskInput) (store.Plan, error) {
					h.workspace.session.current.Current.Fingerprint = "illegal edit"
					return store.Plan{}, billingError()
				}
			}
			if mode == "alternate billing" {
				h.planner.err = nil
				h.implementer.initialErr = billingError()
				alternate.initialErr = billingError()
			}
			result := h.service.Run(ctx, validTask(1))
			if result.Status != store.TaskStatusFailed && result.Status != store.TaskStatusCancelled {
				t.Fatal("unsafe continuation")
			}
			if prompts > 1 {
				t.Fatal("ping-pong fallback")
			}
			if mode != "alternate billing" && result.AgentInvocations != 1 {
				t.Fatal("unsafe extra invocation")
			}
		})
	}
}
