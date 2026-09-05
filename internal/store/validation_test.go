package store

import "testing"

func TestValidationReportValidate(t *testing.T) {
	tests := []struct {
		name      string
		report    ValidationReport
		wantField string
	}{
		{name: "passing report", report: validValidationReport()},
		{name: "empty passing report", report: ValidationReport{Passed: true}},
		{
			name: "failing report",
			report: ValidationReport{
				Checks: []ValidationEvidence{{Command: "go test ./...", ExitCode: 1, Output: "failed"}},
			},
		},
		{
			name: "blank command",
			report: ValidationReport{
				Passed: true,
				Checks: []ValidationEvidence{{Passed: true}},
			},
			wantField: "checks[0].command",
		},
		{
			name: "negative duration",
			report: ValidationReport{
				Passed: true,
				Checks: []ValidationEvidence{
					{Command: "go test ./...", Passed: true, DurationMillis: -1},
				},
			},
			wantField: "checks[0].duration_millis",
		},
		{
			name: "check outcome disagrees with exit code",
			report: ValidationReport{
				Passed: true,
				Checks: []ValidationEvidence{
					{Command: "go test ./...", Passed: true, ExitCode: 1},
				},
			},
			wantField: "checks[0].passed",
		},
		{name: "report outcome disagrees with checks", report: ValidationReport{}, wantField: "passed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertValidationResult(t, test.report.Validate(), test.wantField)
		})
	}
}

func TestValidationRequestValidate(t *testing.T) {
	request := ValidationRequest{
		Input:          validTaskInput(),
		Plan:           validPlan(),
		Implementation: validImplementationResult(),
	}
	invalidRequest := request
	invalidRequest.Implementation.Summary = ""

	assertValidationResult(t, request.Validate(), "")
	assertValidationResult(t, invalidRequest.Validate(), "implementation.summary")
}
