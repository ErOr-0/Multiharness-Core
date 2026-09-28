package workflow_test

import (
	"context"
	"errors"
	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
	"reflect"
	"testing"
)

type permissionResolverFunc func(context.Context, store.WorkflowStage, store.PermissionDenied) (bool, error)

func (f permissionResolverFunc) ResolvePermission(ctx context.Context, stage store.WorkflowStage, denied store.PermissionDenied) (bool, error) {
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
	h.implementer.initialErr = &store.PermissionDenied{Action: store.BlockedAction{Tool: "Write"}, UserDeclined: true}
	s := permissionService(t, h, permissionResolverFunc(func(context.Context, store.WorkflowStage, store.PermissionDenied) (bool, error) {
		t.Fatal("native decline prompted again")
		return true, nil
	}))
	r := s.Run(t.Context(), validTask(1))
	if r.Status != store.TaskStatusNeedsInput || len(h.implementer.implementationCalls) != 1 {
		t.Fatal(r)
	}
}

func TestPermissionRecoveryRetainsPlanBaselineAndPartialWork(t *testing.T) {
	h := newWorkflowHarness(t)
	input := validTask(1)
	input.SessionID = "foreign-planner-session"
	input.RecentTurns = []store.ConversationTurn{{User: "Keep the API compatible", Assistant: "Understood"}}
	denied := &store.PermissionDenied{Action: store.BlockedAction{Tool: "Write", Target: "second.go"}}
	h.implementer.implement = func(_ context.Context, r store.ImplementationRequest) (store.ImplementationResult, error) {
		if !reflect.DeepEqual(r.Input.RecentTurns, input.RecentTurns) || !reflect.DeepEqual(r.Plan, h.planner.plan) || r.Input.SessionID != "" {
			t.Fatal("handoff lost context or reused a foreign session", r)
		}
		if len(h.implementer.implementationCalls) == 1 {
			h.workspace.session.current.Current.Fingerprint = "partial"
			h.workspace.session.current.ChangedFiles = []string{"first.go"}
			return store.ImplementationResult{}, denied
		}
		if r.Repository.Current.Fingerprint != "partial" || r.Repository.Baseline.Fingerprint != "baseline" || !reflect.DeepEqual(r.Repository.ChangedFiles, []string{"first.go"}) {
			t.Fatal("retry lost partial work", r.Repository)
		}
		return implementation("completed", "second.go"), nil
	}
	resolutions := 0
	s := permissionService(t, h, permissionResolverFunc(func(_ context.Context, stage store.WorkflowStage, d store.PermissionDenied) (bool, error) {
		resolutions++
		if stage != store.WorkflowStageImplementation || d.Action != denied.Action || h.workspace.session.closed {
			t.Fatal("lost paused workflow", stage, d)
		}
		return true, nil
	}))
	out := s.Run(t.Context(), input)
	if out.Status != store.TaskStatusApproved || out.AgentInvocations != 4 || resolutions != 1 {
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
			h.implementer.initialErr = &store.PermissionDenied{Action: store.BlockedAction{Tool: "Write"}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			calls := 0
			s := permissionService(t, h, permissionResolverFunc(func(context.Context, store.WorkflowStage, store.PermissionDenied) (bool, error) {
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
			want := store.TaskStatusNeedsInput
			if mode == "cancel" {
				want = store.TaskStatusCancelled
			}
			if mode == "workspace-change" || mode == "resolver-error" {
				want = store.TaskStatusFailed
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
	for _, stage := range []store.WorkflowStage{store.WorkflowStagePlanning, store.WorkflowStageReview, store.WorkflowStageRepair} {
		t.Run(string(stage), func(t *testing.T) {
			h := newWorkflowHarness(t)
			denied := &store.PermissionDenied{Action: store.BlockedAction{Tool: "Read"}}
			wantInvocations := 4
			switch stage {
			case store.WorkflowStagePlanning:
				h.planner.err = denied
			case store.WorkflowStageReview:
				h.reviewer.err = denied
			case store.WorkflowStageRepair:
				h.reviewer.reviews = []store.Review{rejectedReview("fix"), approvedReview("done")}
				h.validator.reports = []store.ValidationReport{passingValidation(), passingValidation()}
				h.implementer.repairErr = denied
				h.implementer.repairs = []store.ImplementationResult{implementation("repaired", "service.go")}
				wantInvocations = 6
			}
			s := permissionService(t, h, permissionResolverFunc(func(_ context.Context, got store.WorkflowStage, _ store.PermissionDenied) (bool, error) {
				if got != stage {
					t.Fatal(got, stage)
				}
				h.planner.err, h.reviewer.err, h.implementer.repairErr = nil, nil, nil
				return true, nil
			}))
			out := s.Run(t.Context(), validTask(1))
			if out.Status != store.TaskStatusApproved || out.AgentInvocations != wantInvocations {
				t.Fatal(out)
			}
			if stage == store.WorkflowStageRepair && out.RepairAttempts != 1 {
				t.Fatal("permission retry consumed repair budget", out)
			}
		})
	}
}

func TestPermissionBlockStopsEveryTeamStageWithEvidence(t *testing.T) {
	for _, stage := range []store.WorkflowStage{store.WorkflowStagePlanning, store.WorkflowStageImplementation, store.WorkflowStageReview, store.WorkflowStageRepair} {
		t.Run(string(stage), func(t *testing.T) {
			h := newWorkflowHarness(t)
			denied := &store.PermissionDenied{SessionID: "ses_blocked", Action: store.BlockedAction{Tool: "read", Target: "/cache/library.go"}}
			wantCalls := 1
			switch stage {
			case store.WorkflowStagePlanning:
				h.planner.err = denied
			case store.WorkflowStageImplementation:
				h.implementer.initialErr = denied
				wantCalls = 2
			case store.WorkflowStageReview:
				h.reviewer.err = denied
				wantCalls = 3
			case store.WorkflowStageRepair:
				h.reviewer.reviews = []store.Review{rejectedReview("repair needed")}
				h.implementer.repairErr = denied
				wantCalls = 4
			}
			out := h.service.Run(t.Context(), validTask(3))
			if out.Status != store.TaskStatusNeedsInput || out.Failure == nil || out.Failure.Code != store.FailureCodePermission || out.Failure.Stage != stage || out.AgentInvocations != wantCalls {
				t.Fatal(out)
			}
			if out.Failure.Permission.SessionID != "ses_blocked" || out.Failure.Permission.Action.Target != "/cache/library.go" {
				t.Fatal(out.Failure)
			}
			if stage == store.WorkflowStageImplementation && (out.Validation != nil || out.LastReview != nil || out.Repository == nil) {
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
	h.implementer.initialErr = errors.Join(&store.PermissionDenied{SessionID: "ses_blocked", Action: store.BlockedAction{Tool: "read"}}, errors.New("workspace inspection failed"))
	out := h.service.Run(t.Context(), validTask(3))
	if out.Status != store.TaskStatusFailed || out.Failure.Permission != nil {
		t.Fatal(out)
	}
}
