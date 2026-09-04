package workflow_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

func TestRunUsesIndependentFilesAndDoesNotExposeMutableEvidence(t *testing.T) {
	h := newWorkflowHarness(t)
	session := newFakeWorkspaceSession()
	session.baseline.PreExistingFiles = []string{"user.txt"}
	session.current = *session.baseline.Clone()
	h.workspace.session = session
	h.implementer.workspace = nil // Model the actual filesystem separately from the claim.
	h.implementer.implement = func(_ context.Context, req store.ImplementationRequest) (store.ImplementationResult, error) {
		if !reflect.DeepEqual(req.Repository.PreExistingFiles, []string{"user.txt"}) {
			t.Fatal("missing protection context")
		}
		req.Repository.PreExistingFiles[0] = "tampered"
		session.current.Current.Fingerprint = "actual-change"
		session.current.ChangedFiles = []string{"actual.go"}
		session.current.Diff = "+actual code"
		return implementation("done", "invented.go"), nil
	}
	output := h.service.Run(t.Context(), validTask(0))
	if output.Status != store.TaskStatusApproved {
		t.Fatalf("output: %#v", output)
	}
	if !reflect.DeepEqual(output.Implementation.ChangedFiles, []string{"actual.go"}) || output.Repository.PreExistingFiles[0] != "user.txt" {
		t.Fatalf("untrusted claim used: %#v", output)
	}
	if h.reviewer.requests[0].Repository.Diff != "+actual code" {
		t.Fatal("reviewer did not receive independent diff")
	}
	if !session.closed {
		t.Fatal("workspace not released")
	}
}

func TestReadOnlyStagesCannotMutateTheValidatedCheckout(t *testing.T) {
	for _, stage := range []store.WorkflowStage{store.WorkflowStagePlanning, store.WorkflowStageValidation, store.WorkflowStageReview} {
		t.Run(string(stage), func(t *testing.T) {
			h := newWorkflowHarness(t)
			h.workspace.session = newFakeWorkspaceSession()
			mutate := func() { h.workspace.session.current.Current.Fingerprint = "unauthorized" }
			switch stage {
			case store.WorkflowStagePlanning:
				h.planner.run = func(context.Context, store.TaskInput) (store.Plan, error) { mutate(); return validPlan(), nil }
			case store.WorkflowStageValidation:
				h.validator.validate = func(context.Context, store.ValidationRequest) (store.ValidationReport, error) {
					mutate()
					return passingValidation(), nil
				}
			case store.WorkflowStageReview:
				h.reviewer.review = func(context.Context, store.ReviewRequest) (store.Review, error) {
					mutate()
					return approvedReview("looks good"), nil
				}
			}
			output := h.service.Run(t.Context(), validTask(0))
			if output.Status != store.TaskStatusFailed || output.Failure.Stage != stage || output.Failure.Code != store.FailureCodeWorkspace {
				t.Fatalf("output: %#v", output)
			}
			if err := output.Validate(); err != nil {
				t.Fatal(err)
			}
			if !h.workspace.session.closed {
				t.Fatal("failed run leaked lease")
			}
		})
	}
}

func TestProtectedUserChangesStopBeforeValidation(t *testing.T) {
	h := newWorkflowHarness(t)
	h.workspace.session = newFakeWorkspaceSession()
	h.implementer.implement = func(context.Context, store.ImplementationRequest) (store.ImplementationResult, error) {
		h.workspace.session.current.PreservationViolations = []string{"user.txt"}
		h.workspace.session.current.RecoveryDirectory = "/recovery"
		return implementation("done", "user.txt"), nil
	}
	output := h.service.Run(t.Context(), validTask(1))
	if output.Status != store.TaskStatusFailed || output.Failure.Code != store.FailureCodeWorkspace || len(h.validator.requests) != 0 {
		t.Fatalf("output: %#v", output)
	}
	if output.Repository.RecoveryDirectory != "/recovery" {
		t.Fatal("recovery location lost")
	}
}

func TestWorkspaceAcquireAndCloseFailuresAreTerminal(t *testing.T) {
	t.Run("acquire", func(t *testing.T) {
		h := newWorkflowHarness(t)
		h.workspace.acquireErr = errors.New("busy")
		output := h.service.Run(t.Context(), validTask(0))
		if output.Status != store.TaskStatusFailed || output.Failure.Stage != store.WorkflowStageIntake || len(h.implementer.implementationCalls) != 0 {
			t.Fatalf("output: %#v", output)
		}
	})
	t.Run("close", func(t *testing.T) {
		h := newWorkflowHarness(t)
		h.workspace.session = newFakeWorkspaceSession()
		h.workspace.session.closeErr = errors.New("release failed")
		output := h.service.Run(t.Context(), validTask(0))
		if output.Status != store.TaskStatusFailed {
			t.Fatal("cleanup failure reported approval")
		}
		for _, event := range h.events.snapshot() {
			if event.Type == workflow.EventTypeWorkflowCompleted && event.Status == store.TaskStatusApproved {
				t.Fatal("emitted approval before cleanup")
			}
		}
	})
}

func TestInspectionErrorMarksEvidenceIncompleteAndRetainsPartialWork(t *testing.T) {
	h := newWorkflowHarness(t)
	h.workspace.session = newFakeWorkspaceSession()
	h.implementer.implement = func(context.Context, store.ImplementationRequest) (store.ImplementationResult, error) {
		h.workspace.session.inspect = func(context.Context) (store.RepositoryEvidence, error) {
			e := h.workspace.session.current
			e.ChangedFiles = []string{"partial.go"}
			e.RecoveryDirectory = "/recovery"
			return e, errors.New("capture failed")
		}
		return implementation("claimed done", "partial.go"), nil
	}
	output := h.service.Run(t.Context(), validTask(0))
	if output.Status != store.TaskStatusFailed || output.Repository.Complete || output.Repository.RecoveryDirectory != "/recovery" {
		t.Fatalf("output: %#v", output)
	}
	if err := output.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidBaselineFailsCleanlyAndReleasesLease(t *testing.T) {
	h := newWorkflowHarness(t)
	h.workspace.session = newFakeWorkspaceSession()
	h.workspace.session.baseline = store.RepositoryEvidence{}
	output := h.service.Run(t.Context(), validTask(0))
	if output.Status != store.TaskStatusFailed || !h.workspace.session.closed {
		t.Fatalf("output: %#v", output)
	}
	if err := output.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestPortPanicDoesNotLeakWorkspaceLease(t *testing.T) {
	h := newWorkflowHarness(t)
	h.workspace.session = newFakeWorkspaceSession()
	h.planner.run = func(context.Context, store.TaskInput) (store.Plan, error) { panic("port panic") }
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
		if !h.workspace.session.closed {
			t.Error("panic leaked lease")
		}
	}()
	_ = h.service.Run(t.Context(), validTask(0))
}
