package workflow_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"multiharness-core/internal/contract"
	"multiharness-core/internal/workflow"
)

type permissionResolverFunc func(context.Context, contract.WorkflowStage, contract.PermissionDenied) (bool, error)

func (f permissionResolverFunc) ResolvePermission(ctx context.Context, stage contract.WorkflowStage, denied contract.PermissionDenied) (bool, error) {
	return f(ctx, stage, denied)
}

func permissionService(t *testing.T, h *workflowHarness, resolver workflow.PermissionResolver) *workflow.Service {
	t.Helper()
	s, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Validator: h.validator, Reviewer: h.reviewer, PermissionResolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNativeDeclineDoesNotAskForRecoveryOrReplay(t *testing.T) {
	h := newWorkflowHarness(t)
	h.implementer.initialErr = &contract.PermissionDenied{Action: contract.BlockedAction{Tool: "Write"}, UserDeclined: true}
	s := permissionService(t, h, permissionResolverFunc(func(context.Context, contract.WorkflowStage, contract.PermissionDenied) (bool, error) {
		t.Fatal("native decline prompted again")
		return true, nil
	}))
	r := s.Run(t.Context(), validTask(1))
	if r.Status != contract.TaskStatusNeedsInput || len(h.implementer.implementationCalls) != 1 {
		t.Fatal(r)
	}
}

func TestPermissionRecoveryRetainsPlanBaselineAndPartialWork(t *testing.T) {
	h := newWorkflowHarness(t)
	input := validTask(1)
	input.SessionID = "foreign-planner-session"
	input.RecentTurns = []contract.ConversationTurn{{User: "Keep the API compatible", Assistant: "Understood"}}
	denied := &contract.PermissionDenied{Action: contract.BlockedAction{Tool: "Write", Target: "second.go"}}
	h.implementer.implement = func(_ context.Context, r contract.ImplementationRequest) (contract.ImplementationResult, error) {
		if !reflect.DeepEqual(r.Input.RecentTurns, input.RecentTurns) || !reflect.DeepEqual(r.Plan, h.planner.plan) || r.Input.SessionID != "" {
			t.Fatal("handoff lost context or reused a foreign session", r)
		}
		if len(h.implementer.implementationCalls) == 1 {
			h.workspace.session.current.Current.Fingerprint = "partial"
			h.workspace.session.current.ChangedFiles = []string{"first.go"}
			return contract.ImplementationResult{}, denied
		}
		if r.Repository.Current.Fingerprint != "partial" || r.Repository.Baseline.Fingerprint != "baseline" || !reflect.DeepEqual(r.Repository.ChangedFiles, []string{"first.go"}) {
			t.Fatal("retry lost partial work", r.Repository)
		}
		return implementation("completed", "second.go"), nil
	}
	resolutions := 0
	s := permissionService(t, h, permissionResolverFunc(func(_ context.Context, stage contract.WorkflowStage, d contract.PermissionDenied) (bool, error) {
		resolutions++
		if stage != contract.WorkflowStageImplementation || d.Action != denied.Action || h.workspace.session.closed {
			t.Fatal("lost paused workflow", stage, d)
		}
		return true, nil
	}))
	out := s.Run(t.Context(), input)
	if out.Status != contract.TaskStatusApproved || out.AgentInvocations != 4 || resolutions != 1 {
		t.Fatal(out, resolutions)
	}
	if got := h.calls.snapshot(); !reflect.DeepEqual(got, []string{"plan", "workspace", "implement", "implement", "validate", "review"}) {
		t.Fatal("replayed completed stages", got)
	}
}

func TestPermissionRecoveryStopsOnRefusalCancellationAndRepeatedDenial(t *testing.T) {
	for _, mode := range []string{"no", "cancel", "repeat", "workspace-change", "resolver-error"} {
		t.Run(mode, func(t *testing.T) {
			h := newWorkflowHarness(t)
			h.implementer.initialErr = &contract.PermissionDenied{Action: contract.BlockedAction{Tool: "Write"}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			s := permissionService(t, h, permissionResolverFunc(func(context.Context, contract.WorkflowStage, contract.PermissionDenied) (bool, error) {
				calls++
				switch mode {
				case "no":
					return false, nil
				case "cancel":
					cancel()
				case "workspace-change":
					h.workspace.session.current.Current.Fingerprint = "external-change"
				case "resolver-error":
					return true, errors.New("terminal failed")
				}
				return true, nil
			}))
			out := s.Run(ctx, validTask(1))
			want := contract.TaskStatusNeedsInput
			if mode == "cancel" {
				want = contract.TaskStatusCancelled
			}
			if mode == "workspace-change" || mode == "resolver-error" {
				want = contract.TaskStatusFailed
			}
			if out.Status != want || out.Validation != nil || out.LastReview != nil || !h.workspace.session.closed {
				t.Fatal(mode, out)
			}
			wantCalls, wantInvocations := 1, 2
			if mode == "repeat" {
				wantCalls, wantInvocations = 3, 5
			}
			if calls != wantCalls || out.AgentInvocations != wantInvocations {
				t.Fatal(calls, out.AgentInvocations)
			}
		})
	}
}

func TestPermissionRecoveryRetriesOnlyBlockedRole(t *testing.T) {
	for _, stage := range []contract.WorkflowStage{contract.WorkflowStagePlanning, contract.WorkflowStageReview, contract.WorkflowStageRepair} {
		t.Run(string(stage), func(t *testing.T) {
			h := newWorkflowHarness(t)
			denied := &contract.PermissionDenied{Action: contract.BlockedAction{Tool: "Read"}}
			wantInvocations := 4
			switch stage {
			case contract.WorkflowStagePlanning:
				h.planner.err = denied
			case contract.WorkflowStageReview:
				h.reviewer.err = denied
			case contract.WorkflowStageRepair:
				h.reviewer.reviews = []contract.Review{rejectedReview("fix"), approvedReview("done")}
				h.validator.reports = []contract.ValidationReport{passingValidation(), passingValidation()}
				h.implementer.repairErr = denied
				h.implementer.repairs = []contract.ImplementationResult{implementation("repaired", "service.go")}
				wantInvocations = 6
			}
			s := permissionService(t, h, permissionResolverFunc(func(_ context.Context, got contract.WorkflowStage, _ contract.PermissionDenied) (bool, error) {
				if got != stage {
					t.Fatal(got, stage)
				}
				h.planner.err, h.reviewer.err, h.implementer.repairErr = nil, nil, nil
				return true, nil
			}))
			out := s.Run(t.Context(), validTask(1))
			if out.Status != contract.TaskStatusApproved || out.AgentInvocations != wantInvocations {
				t.Fatal(out)
			}
			if stage == contract.WorkflowStageRepair && out.RepairAttempts != 1 {
				t.Fatal("permission retry consumed repair budget", out)
			}
		})
	}
}

func TestPermissionBlockStopsEveryTeamStageWithEvidence(t *testing.T) {
	for _, stage := range []contract.WorkflowStage{contract.WorkflowStagePlanning, contract.WorkflowStageImplementation, contract.WorkflowStageReview, contract.WorkflowStageRepair} {
		t.Run(string(stage), func(t *testing.T) {
			h := newWorkflowHarness(t)
			denied := &contract.PermissionDenied{SessionID: "ses_blocked", Action: contract.BlockedAction{Tool: "read", Target: "/cache/library.go"}}
			wantCalls := 1
			switch stage {
			case contract.WorkflowStagePlanning:
				h.planner.err = denied
			case contract.WorkflowStageImplementation:
				h.implementer.initialErr = denied
				wantCalls = 2
			case contract.WorkflowStageReview:
				h.reviewer.err = denied
				wantCalls = 3
			case contract.WorkflowStageRepair:
				h.reviewer.reviews = []contract.Review{rejectedReview("repair needed")}
				h.implementer.repairErr = denied
				wantCalls = 4
			}
			out := h.service.Run(t.Context(), validTask(3))
			if out.Status != contract.TaskStatusNeedsInput || out.Failure == nil || out.Failure.Code != contract.FailureCodePermission || out.Failure.Stage != stage || out.AgentInvocations != wantCalls {
				t.Fatal(out)
			}
			if out.Failure.Permission.SessionID != "ses_blocked" || out.Failure.Permission.Action.Target != "/cache/library.go" {
				t.Fatal(out.Failure)
			}
			if stage == contract.WorkflowStageImplementation && (out.Validation != nil || out.LastReview != nil || out.Repository == nil) {
				t.Fatal("advanced past block or lost recovery evidence", out)
			}
			if err := out.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPermissionDenialDoesNotHideAnotherFailure(t *testing.T) {
	h := newWorkflowHarness(t)
	h.implementer.initialErr = errors.Join(&contract.PermissionDenied{SessionID: "ses_blocked", Action: contract.BlockedAction{Tool: "read"}}, errors.New("workspace inspection failed"))
	out := h.service.Run(t.Context(), validTask(3))
	if out.Status != contract.TaskStatusFailed || out.Failure.Permission != nil {
		t.Fatal(out)
	}
}
