package workflow_test

import (
	"context"
	"errors"
	"testing"

	"multiharness-core/internal/store"
	"multiharness-core/internal/workflow"
)

type validationConsent func(context.Context, string, store.ValidationAction) (bool, error)

func (f validationConsent) ConfirmValidation(c context.Context, dir string, a store.ValidationAction) (bool, error) {
	return f(c, dir, a)
}

type actionValidator struct {
	*fakeValidator
	run func(context.Context, store.ValidationRequest, store.ValidationAction) (store.ValidationReport, error)
}

func (v actionValidator) ValidateAction(c context.Context, r store.ValidationRequest, a store.ValidationAction) (store.ValidationReport, error) {
	return v.run(c, r, a)
}

func TestValidationRequestConsentAndResume(t *testing.T) {
	for _, mode := range []string{"yes", "no", "unattended", "cancel", "repeat", "failed", "changed_during_prompt", "write_artifacts"} {
		t.Run(mode, func(t *testing.T) {
			h := newWorkflowHarness(t)
			r := rejectedReview("build requires writable cache")
			r.ValidationAction = &store.ValidationAction{Executable: "go", Args: []string{"test", "./..."}, Reason: "Run tests with writable build cache"}
			h.reviewer.reviews = []store.Review{r, approvedReview("verified")}
			if mode == "repeat" {
				h.reviewer.reviews[1] = r
			}
			h.validator.reports = []store.ValidationReport{{Passed: true}, passingValidation()}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			prompts, runs := 0, 0
			var approve workflow.ValidationApprover = validationConsent(func(_ context.Context, dir string, a store.ValidationAction) (bool, error) {
				prompts++
				if dir != validTask(0).WorkingDir || a.Executable != "go" {
					t.Fatal(dir, a)
				}
				if mode == "cancel" {
					cancel()
				}
				if mode == "changed_during_prompt" {
					h.workspace.session.current.Current.Fingerprint = "external change"
				}
				return mode != "no", nil
			})
			if mode == "unattended" {
				approve = nil
			}
			v := actionValidator{h.validator, func(_ context.Context, request store.ValidationRequest, a store.ValidationAction) (store.ValidationReport, error) {
				runs++
				if mode == "failed" {
					return failingValidation(), nil
				}
				if mode == "write_artifacts" {
					h.workspace.session.current.Current.Fingerprint = "test artifacts"
					h.workspace.session.current.ChangedFiles = append(h.workspace.session.current.ChangedFiles, "go.sum")
				}
				return passingValidation(), nil
			}}
			s, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Reviewer: h.reviewer, Validator: v, ValidationApprover: approve})
			if err != nil {
				t.Fatal(err)
			}
			out := s.Run(ctx, validTask(3))
			if out.RepairAttempts != 0 || len(h.implementer.repairCalls) != 0 {
				t.Fatal("spent repair attempts", out)
			}
			if err := out.Validate(); err != nil {
				t.Fatal(err, out)
			}
			switch mode {
			case "yes", "write_artifacts":
				if out.Status != store.TaskStatusApproved || runs != 1 || prompts != 1 || len(h.reviewer.requests) != 2 || len(h.reviewer.requests[1].Validation.Checks) != 2 {
					t.Fatal(out, runs, prompts)
				}
			case "no", "unattended":
				if out.Status != store.TaskStatusNeedsInput || runs != 0 {
					t.Fatal(out, runs)
				}
			case "repeat":
				if out.Status != store.TaskStatusNeedsInput || runs != 1 || prompts != 1 {
					t.Fatal(out, runs, prompts)
				}
			case "cancel":
				if out.Status != store.TaskStatusCancelled || runs != 0 {
					t.Fatal(out, runs)
				}
			case "failed", "changed_during_prompt":
				if out.Status != store.TaskStatusFailed {
					t.Fatal(out)
				}
				if mode == "changed_during_prompt" && runs != 0 {
					t.Fatal("ran stale command")
				}
			}
		})
	}
}

func TestValidationConsentErrorDoesNotExecute(t *testing.T) {
	h := newWorkflowHarness(t)
	r := rejectedReview("needs checks")
	r.ValidationAction = &store.ValidationAction{Executable: "go", Args: []string{}, Reason: "verify"}
	h.reviewer.reviews = []store.Review{r}
	v := actionValidator{h.validator, func(context.Context, store.ValidationRequest, store.ValidationAction) (store.ValidationReport, error) {
		t.Fatal("executed after input error")
		return passingValidation(), nil
	}}
	s, err := workflow.NewService(workflow.Dependencies{Workspace: h.workspace, Planner: h.planner, Implementer: h.implementer, Reviewer: h.reviewer, Validator: v, ValidationApprover: validationConsent(func(context.Context, string, store.ValidationAction) (bool, error) {
		return true, errors.New("input failed")
	})})
	if err != nil {
		t.Fatal(err)
	}
	if out := s.Run(t.Context(), validTask(3)); out.Status != store.TaskStatusFailed || out.RepairAttempts != 0 {
		t.Fatal(out)
	}
}
