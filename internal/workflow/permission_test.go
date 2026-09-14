package workflow_test

import (
	"errors"
	"multiharness-core/internal/store"
	"testing"
)

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
