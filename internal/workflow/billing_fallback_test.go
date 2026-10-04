package workflow_test

import (
	"context"
	"errors"
	"testing"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

type approvalFunc func(context.Context, contract.AgentSwitch) (bool, error)

func (f approvalFunc) ConfirmFallback(ctx context.Context, r contract.AgentSwitch) (bool, error) {
	return f(ctx, r)
}
func billingError() error {
	return &contract.ProviderFailure{Kind: contract.ProviderBillingExhausted, Attempts: 1}
}
func installFallback(t *testing.T, h *workflowHarness, approval workflow.BillingApprover, limit int) *fakeImplementer {
	t.Helper()
	alternate := &fakeImplementer{
		workspace: h.workspace,
		initial:   implementation("alternate", "service.go"),
		repairs:   []contract.ImplementationResult{implementation("fixed", "service.go")},
	}
	f := workflow.BillingFallbacks{
		Planner:        &fakePlanner{plan: validPlan()},
		Implementer:    alternate,
		Reviewer:       &fakeReviewer{reviews: []contract.Review{approvedReview("alternate approved")}},
		Approver:       approval,
		Planning:       contract.AgentSwitch{Stage: contract.WorkflowStagePlanning, From: "Primary", To: "Alternate", Model: "model"},
		Review:         contract.AgentSwitch{Stage: contract.WorkflowStageReview, From: "Primary", To: "Alternate", Model: "model"},
		Implementation: contract.AgentSwitch{Stage: contract.WorkflowStageImplementation, From: "Primary", To: "Alternate", Model: "model", CanWrite: true},
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
	for _, stage := range []contract.WorkflowStage{contract.WorkflowStagePlanning, contract.WorkflowStageImplementation, contract.WorkflowStageReview, contract.WorkflowStageRepair} {
		for _, yes := range []bool{false, true} {
			t.Run(string(stage)+map[bool]string{true: "/yes", false: "/no"}[yes], func(t *testing.T) {
				h := newWorkflowHarness(t)
				prompts := 0
				alternate := installFallback(t, h, approvalFunc(func(_ context.Context, choice contract.AgentSwitch) (bool, error) {
					prompts++
					if choice.Stage != stage {
						t.Fatal("wrong role")
					}
					return yes, nil
				}), 20)
				switch stage {
				case contract.WorkflowStagePlanning:
					h.planner.err = billingError()
				case contract.WorkflowStageImplementation:
					h.implementer.initialErr = billingError()
				case contract.WorkflowStageReview:
					h.reviewer.err = billingError()
				case contract.WorkflowStageRepair:
					h.reviewer.reviews = []contract.Review{rejectedReview("repair"), approvedReview("done")}
					h.validator.reports = []contract.ValidationReport{failingValidation(), passingValidation()}
					h.implementer.repairErr = billingError()
				}
				result := h.service.Run(t.Context(), validTask(2))
				if prompts != 1 {
					t.Fatalf("prompts=%d", prompts)
				}
				if yes && (result.Status != contract.TaskStatusApproved || len(result.AgentSwitches) != 1) {
					t.Fatalf("switch failed: %+v", result.Failure)
				}
				if !yes && (result.Status != contract.TaskStatusFailed || len(result.AgentSwitches) != 0) {
					t.Fatal("decline continued")
				}
				if stage == contract.WorkflowStageRepair && yes && alternate.repairCalls[0].Implementation.AgentSessionID != "" {
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
	alternate := installFallback(t, h, approvalFunc(func(context.Context, contract.AgentSwitch) (bool, error) { prompts++; return true, nil }), 20)
	h.implementer.implement = func(context.Context, contract.ImplementationRequest) (contract.ImplementationResult, error) {
		h.workspace.session.current.Current.Fingerprint = "partial"
		h.workspace.session.current.ChangedFiles = []string{"partial.go"}
		return contract.ImplementationResult{}, billingError()
	}
	alternate.implement = func(_ context.Context, r contract.ImplementationRequest) (contract.ImplementationResult, error) {
		if r.Repository.Current.Fingerprint != "partial" || len(r.Plan.Steps) == 0 {
			t.Fatal("lost partial evidence or plan")
		}
		return implementation("continued", "service.go"), nil
	}
	h.reviewer.reviews = []contract.Review{rejectedReview("repair"), approvedReview("fixed")}
	h.validator.reports = []contract.ValidationReport{failingValidation(), passingValidation()}
	result := h.service.Run(t.Context(), validTask(1))
	if result.Status != contract.TaskStatusApproved || prompts != 1 || len(alternate.repairCalls) != 1 || len(h.implementer.repairCalls) != 0 || result.AgentInvocations != 6 {
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
			var approval workflow.BillingApprover = approvalFunc(func(context.Context, contract.AgentSwitch) (bool, error) {
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
			if mode == "prompt mutation" {
				h.planner.err = nil
				h.reviewer.err = billingError()
			}
			if mode == "read-only mutation" {
				h.planner.err = nil
				h.reviewer.review = func(context.Context, contract.ReviewRequest) (contract.Review, error) {
					h.workspace.session.current.Current.Fingerprint = "illegal edit"
					return contract.Review{}, billingError()
				}
			}
			if mode == "alternate billing" {
				h.planner.err = nil
				h.implementer.initialErr = billingError()
				alternate.initialErr = billingError()
			}
			result := h.service.Run(ctx, validTask(1))
			if result.Status != contract.TaskStatusFailed && result.Status != contract.TaskStatusCancelled {
				t.Fatal("unsafe continuation")
			}
			if prompts > 1 {
				t.Fatal("ping-pong fallback")
			}
			if mode != "alternate billing" && mode != "prompt mutation" && mode != "read-only mutation" && result.AgentInvocations != 1 {
				t.Fatal("unsafe extra invocation")
			}
		})
	}
}
