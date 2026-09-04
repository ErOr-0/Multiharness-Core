package store

import "testing"

func TestPlanValidate(t *testing.T) {
	tests := []struct {
		name      string
		plan      Plan
		wantField string
	}{
		{name: "valid", plan: validPlan()},
		{
			name:      "blank summary",
			plan:      Plan{Steps: []string{"implement"}, AcceptanceCriteria: []string{"tests pass"}},
			wantField: "summary",
		},
		{
			name:      "missing steps",
			plan:      Plan{Action: PlanActionImplement, Summary: "Implement change", AcceptanceCriteria: []string{"tests pass"}},
			wantField: "steps",
		},
		{
			name: "blank acceptance criterion",
			plan: Plan{
				Action:             PlanActionImplement,
				Summary:            "Implement change",
				Steps:              []string{"implement"},
				AcceptanceCriteria: []string{" "},
			},
			wantField: "acceptance_criteria[0]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertValidationResult(t, test.plan.Validate(), test.wantField)
		})
	}
}

func TestPlanJSON(t *testing.T) {
	assertJSONRoundTrip(t, validPlan())
}

func TestAnswerPlanContract(t *testing.T) {
	answer := Plan{Action: PlanActionAnswer, Summary: "Explanation", Answer: "Here is the answer."}
	if err := answer.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := answer.ValidateImplementation(); err == nil {
		t.Fatal("answer crossed implementation boundary")
	}
	assertJSONRoundTrip(t, answer)
	for _, mutate := range []func(*Plan){
		func(p *Plan) { p.Action = "" }, func(p *Plan) { p.Action = "unknown" }, func(p *Plan) { p.Answer = " " },
		func(p *Plan) { p.Steps = []string{"do work"} }, func(p *Plan) { p.AcceptanceCriteria = []string{"test"} },
	} {
		invalid := answer
		mutate(&invalid)
		if err := invalid.Validate(); err == nil {
			t.Fatalf("accepted invalid answer: %#v", invalid)
		}
	}
	implementation := validPlan()
	implementation.Answer = "conflicting answer"
	if err := implementation.Validate(); err == nil {
		t.Fatal("implementation included an answer")
	}
}
