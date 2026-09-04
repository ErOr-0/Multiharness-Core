package structured

import (
	"fmt"

	"multiharness-core/internal/store"
)

func ParsePlan(data []byte) (store.Plan, error) {
	var response planResponse
	if err := decodeStrict(data, &response); err != nil {
		return store.Plan{}, &OutputError{Role: rolePlanning, Cause: err}
	}
	if err := requirePlanFields(response); err != nil {
		return store.Plan{}, &OutputError{Role: rolePlanning, Cause: err}
	}
	if *response.SchemaVersion != planSchemaVersion {
		return store.Plan{}, &OutputError{
			Role:  rolePlanning,
			Cause: fmt.Errorf("unsupported schema_version %q", *response.SchemaVersion),
		}
	}

	plan := store.Plan{
		Action:             *response.Action,
		Answer:             *response.Answer,
		Summary:            *response.Summary,
		Steps:              *response.Steps,
		AcceptanceCriteria: *response.AcceptanceCriteria,
	}
	if err := plan.Validate(); err != nil {
		return store.Plan{}, &OutputError{Role: rolePlanning, Cause: err}
	}
	return plan, nil
}

func ParseReview(data []byte) (store.Review, error) {
	var response reviewResponse
	if err := decodeStrict(data, &response); err != nil {
		return store.Review{}, &OutputError{Role: roleReview, Cause: err}
	}
	if err := requireReviewFields(response); err != nil {
		return store.Review{}, &OutputError{Role: roleReview, Cause: err}
	}
	if *response.SchemaVersion != reviewSchemaVersion {
		return store.Review{}, &OutputError{
			Role:  roleReview,
			Cause: fmt.Errorf("unsupported schema_version %q", *response.SchemaVersion),
		}
	}

	findings := make([]store.ReviewFinding, 0, len(*response.Findings))
	for _, finding := range *response.Findings {
		findings = append(findings, store.ReviewFinding{
			Severity:       *finding.Severity,
			Blocking:       *finding.Blocking,
			File:           *finding.File,
			Line:           *finding.Line,
			Description:    *finding.Description,
			Evidence:       *finding.Evidence,
			RequiredAction: *finding.RequiredAction,
		})
	}
	review := store.Review{
		Approved:    *response.Approved,
		Summary:     *response.Summary,
		Findings:    findings,
		Suggestions: *response.Suggestions,
	}
	if err := review.Validate(); err != nil {
		return store.Review{}, &OutputError{Role: roleReview, Cause: err}
	}
	return review, nil
}

func requirePlanFields(response planResponse) error {
	missing := ""
	switch {
	case response.SchemaVersion == nil:
		missing = "schema_version"
	case response.Action == nil:
		missing = "action"
	case response.Answer == nil:
		missing = "answer"
	case response.Summary == nil:
		missing = "summary"
	case response.Steps == nil:
		missing = "steps"
	case response.AcceptanceCriteria == nil:
		missing = "acceptance_criteria"
	}
	if missing != "" {
		return fmt.Errorf("required field %q is missing or null", missing)
	}
	return nil
}

func requireReviewFields(response reviewResponse) error {
	missing := ""
	switch {
	case response.SchemaVersion == nil:
		missing = "schema_version"
	case response.Approved == nil:
		missing = "approved"
	case response.Summary == nil:
		missing = "summary"
	case response.Findings == nil:
		missing = "findings"
	case response.Suggestions == nil:
		missing = "suggestions"
	}
	if missing != "" {
		return fmt.Errorf("required field %q is missing or null", missing)
	}
	for index, finding := range *response.Findings {
		if field := missingReviewFindingField(finding); field != "" {
			return fmt.Errorf(
				"required field %q is missing or null",
				fmt.Sprintf("findings[%d].%s", index, field),
			)
		}
	}
	return nil
}

func missingReviewFindingField(finding reviewFindingResponse) string {
	switch {
	case finding.Severity == nil:
		return "severity"
	case finding.Blocking == nil:
		return "blocking"
	case finding.File == nil:
		return "file"
	case finding.Line == nil:
		return "line"
	case finding.Description == nil:
		return "description"
	case finding.Evidence == nil:
		return "evidence"
	case finding.RequiredAction == nil:
		return "required_action"
	default:
		return ""
	}
}
