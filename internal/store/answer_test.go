package store

import "testing"

func TestAnsweredOutputRequiresUnchangedEvidenceAndNoCoding(t *testing.T) {
	state := RepositoryState{Root: "/repo", Fingerprint: "original"}
	base := TaskOutput{
		Status:     TaskStatusAnswered,
		Summary:    "Answer",
		Plan:       &Plan{Action: PlanActionAnswer, Summary: "Summary", Answer: "Answer"},
		Repository: &RepositoryEvidence{Baseline: state, Current: state, Complete: true},
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*TaskOutput){
		func(o *TaskOutput) { o.Plan = nil },
		func(o *TaskOutput) { o.Plan.Action = PlanActionImplement },
		func(o *TaskOutput) { o.Plan.Answer = "" },
		func(o *TaskOutput) { o.Summary = "different" },
		func(o *TaskOutput) { o.Implementation = &ImplementationResult{} },
		func(o *TaskOutput) { o.Validation = &ValidationReport{} },
		func(o *TaskOutput) { o.LastReview = &Review{} },
		func(o *TaskOutput) { o.RepairAttempts = 1 },
		func(o *TaskOutput) { o.Repository = nil },
		func(o *TaskOutput) { o.Repository.Complete = false },
		func(o *TaskOutput) { o.Repository.Current.Fingerprint = "changed" },
		func(o *TaskOutput) { o.Repository.ChangedFiles = []string{"file"} },
		func(o *TaskOutput) { o.Repository.PreservationViolations = []string{"file"} },
	} {
		output := base
		plan := *base.Plan
		output.Plan = &plan
		output.Repository = base.Repository.Clone()
		mutate(&output)
		if err := output.Validate(); err == nil {
			t.Fatalf("invalid answered output accepted: %#v", output)
		}
	}
}
