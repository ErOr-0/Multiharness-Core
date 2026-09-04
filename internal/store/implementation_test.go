package store

import "testing"

func TestImplementationRequestValidate(t *testing.T) {
	request := ImplementationRequest{Input: validTaskInput(), Plan: validPlan()}
	invalidRequest := request
	invalidRequest.Plan.Summary = ""

	assertValidationResult(t, request.Validate(), "")
	assertValidationResult(t, invalidRequest.Validate(), "plan.summary")
}

func TestImplementationResultValidate(t *testing.T) {
	tests := []struct {
		name      string
		result    ImplementationResult
		wantField string
	}{
		{name: "valid", result: validImplementationResult()},
		{name: "blank summary", result: ImplementationResult{}, wantField: "summary"},
		{
			name:      "blank changed file",
			result:    ImplementationResult{Summary: "done", ChangedFiles: []string{" "}},
			wantField: "changed_files[0]",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertValidationResult(t, test.result.Validate(), test.wantField)
		})
	}
}

func TestImplementationJSON(t *testing.T) {
	assertJSONRoundTrip(t, ImplementationRequest{Input: validTaskInput(), Plan: validPlan()})
	assertJSONRoundTrip(t, validImplementationResult())
}
