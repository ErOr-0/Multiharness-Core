package store

import (
	"fmt"
	"strings"
)

type ValidationEvidence struct {
	Command         string `json:"command"`
	Passed          bool   `json:"passed"`
	ExitCode        int    `json:"exit_code"`
	Output          string `json:"output"`
	DurationMillis  int64  `json:"duration_millis"`
	OutputTruncated bool   `json:"output_truncated"`
}

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

type ValidationReport struct {
	Passed bool                 `json:"passed"`
	Checks []ValidationEvidence `json:"checks"`
}

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

type ValidationRequest struct {
	Repository     *RepositoryEvidence  `json:"repository,omitempty"`
	Input          TaskInput            `json:"input"`
	Plan           Plan                 `json:"plan"`
	Implementation ImplementationResult `json:"implementation"`
}

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
