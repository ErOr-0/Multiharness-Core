package store

import "testing"

func TestDirectResultsCannotClaimTeamApproval(t *testing.T) {
	valid := TaskOutput{Status: TaskStatusResponded, Summary: "Native response", Direct: &DirectResponse{Text: "Native response", SessionID: "ses_1"}, AgentInvocations: 1}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*TaskOutput){
		func(o *TaskOutput) { o.Status = TaskStatusApproved },
		func(o *TaskOutput) { o.Validation = &ValidationReport{Passed: true} },
		func(o *TaskOutput) {
			o.Plan = &Plan{Action: PlanActionAnswer, Answer: "Native response", Summary: "answer"}
		},
		func(o *TaskOutput) { o.Direct = nil },
		func(o *TaskOutput) { o.Summary = "invented summary" },
		func(o *TaskOutput) { o.Status = TaskStatusNeedsInput },
	} {
		invalid := valid
		mutate(&invalid)
		if err := invalid.Validate(); err == nil {
			t.Fatalf("accepted invalid evidence: %+v", invalid)
		}
	}
}
