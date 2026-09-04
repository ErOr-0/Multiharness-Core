package store

import (
	"fmt"
	"strings"
)

// ValidationEvidence records the outcome of one deterministic validation
// command. DurationMillis is used instead of time.Duration to keep the JSON
// representation explicit and language-neutral.
type ValidationEvidence struct {
	Command         string `json:"command"`
	Passed          bool   `json:"passed"`
	ExitCode        int    `json:"exit_code"`
	Output          string `json:"output"`
	DurationMillis  int64  `json:"duration_millis"`
	OutputTruncated bool   `json:"output_truncated"`
}

// Validate checks one deterministic validation command result.
func (evidence ValidationEvidence) Validate() error {
	if strings.TrimSpace(evidence.Command) == "" {
		return invalid("command", "must not be blank")
	}
	if evidence.DurationMillis < 0 {
		return invalid("duration_millis", "must be zero or greater")
	}
	if evidence.Passed != (evidence.ExitCode == 0) {
		return invalid("passed", "must agree with whether exit_code is zero")
	}
	return nil
}

// ValidationReport is produced independently from the implementation agent.
// Passed must agree with the outcomes in Checks.
type ValidationReport struct {
	Passed bool                 `json:"passed"`
	Checks []ValidationEvidence `json:"checks"`
}

// Validate checks that the report agrees with all of its command evidence.
// An empty report is valid only when Passed is true, representing no configured
// deterministic checks rather than a failed check with missing evidence.
func (report ValidationReport) Validate() error {
	allPassed := true
	for i, evidence := range report.Checks {
		if err := evidence.Validate(); err != nil {
			return nested(fmt.Sprintf("checks[%d]", i), err)
		}
		if !evidence.Passed {
			allPassed = false
		}
	}
	if report.Passed != allPassed {
		return invalid("passed", "must agree with all validation checks")
	}
	return nil
}

// ValidationRequest contains the workflow state needed by a validator.
type ValidationRequest struct {
	Repository     *RepositoryEvidence  `json:"repository,omitempty"`
	Input          TaskInput            `json:"input"`
	Plan           Plan                 `json:"plan"`
	Implementation ImplementationResult `json:"implementation"`
}

// Validate checks a deterministic validation request.
func (request ValidationRequest) Validate() error {
	if err := request.Input.Validate(); err != nil {
		return nested("input", err)
	}
	if err := request.Plan.ValidateImplementation(); err != nil {
		return nested("plan", err)
	}
	if err := request.Implementation.Validate(); err != nil {
		return nested("implementation", err)
	}
	return validateRepository(request.Repository)
}
