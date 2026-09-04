package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"multiharness-core/internal/store"
)

func TestRunReturnsEmptyEvidenceCollectionsAsJSONArrays(t *testing.T) {
	harness := newWorkflowHarness(t)
	harness.implementer.initial = store.ImplementationResult{Summary: "no changes needed"}
	harness.validator.reports = []store.ValidationReport{{Passed: true}}
	reviewResults := []store.Review{{Approved: true, Summary: "verified"}}
	harness.reviewer.reviews = reviewResults
	harness.workspace.session = newFakeWorkspaceSession()
	harness.workspace.session.baseline.PreExistingFiles = nil
	harness.workspace.session.current.PreExistingFiles = nil
	harness.workspace.session.current.PreservationViolations = nil

	output := harness.service.Run(t.Context(), validTask(0))
	if output.Status != store.TaskStatusApproved {
		t.Fatalf("Run() output = %#v, want approval", output)
	}
	data := marshalOutput(t, output)
	for _, field := range []string{"changed_files", "pre_existing_files", "preservation_violations", "checks", "findings", "suggestions"} {
		if !strings.Contains(data, `"`+field+`":[]`) {
			t.Errorf("empty %s must be an array: %s", field, data)
		}
	}
	if strings.Count(data, `"changed_files":[]`) != 2 {
		t.Fatalf("repository and implementation must both report empty changes: %s", data)
	}
	if reviewResults[0].Findings != nil || reviewResults[0].Suggestions != nil {
		t.Fatal("output normalization mutated the reviewer's result")
	}
}

func TestRunPreservesJSONShapeForPartialAndTerminalResults(t *testing.T) {
	for _, status := range []store.TaskStatus{store.TaskStatusFailed, store.TaskStatusCancelled, store.TaskStatusRepairLimitReached} {
		t.Run(string(status), func(t *testing.T) {
			harness := newWorkflowHarness(t)
			harness.validator.reports = []store.ValidationReport{{Passed: true}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			switch status {
			case store.TaskStatusFailed:
				harness.reviewer.err = errors.New("review unavailable")
			case store.TaskStatusCancelled:
				harness.reviewer.review = func(context.Context, store.ReviewRequest) (store.Review, error) {
					cancel()
					return store.Review{}, ctx.Err()
				}
			case store.TaskStatusRepairLimitReached:
				harness.reviewer.reviews = []store.Review{rejectedReview("repair required")}
			}
			output := harness.service.Run(ctx, validTask(0))
			if output.Status != status {
				t.Fatalf("Run() status = %q, want %q", output.Status, status)
			}
			data := marshalOutput(t, output)
			if !strings.Contains(data, `"checks":[]`) {
				t.Fatalf("completed validation must retain its empty array: %s", data)
			}
			if status == store.TaskStatusRepairLimitReached {
				if !strings.Contains(data, `"suggestions":[]`) || output.LastReview == nil || len(output.LastReview.Findings) != 1 {
					t.Fatalf("rejected review evidence changed: %s", data)
				}
			} else if strings.Contains(data, `"last_review"`) {
				t.Fatalf("missing review must remain omitted: %s", data)
			}
		})
	}
}

func TestRunOmitsUnproducedEvidenceFromJSON(t *testing.T) {
	harness := newWorkflowHarness(t)
	output := harness.service.Run(t.Context(), store.TaskInput{})
	if output.Status != store.TaskStatusFailed {
		t.Fatalf("Run() status = %q, want failed", output.Status)
	}
	data := marshalOutput(t, output)
	for _, field := range []string{"repository", "plan", "implementation", "validation", "last_review"} {
		if strings.Contains(data, `"`+field+`":`) {
			t.Errorf("unproduced %s evidence must be omitted: %s", field, data)
		}
	}
}

func marshalOutput(t *testing.T, output store.TaskOutput) string {
	t.Helper()
	if err := output.Validate(); err != nil {
		t.Fatalf("Run() returned invalid output: %v", err)
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	var decoded store.TaskOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("JSON round trip violated the output contract: %v", err)
	}
	return string(data)
}
