package workflow

import "multiharness-core/internal/store"

// normalizeTaskOutput preserves domain meaning while keeping empty collections
// in returned evidence serializable as JSON arrays rather than null. Optional
// evidence pointers stay nil when their stage has not produced a result.
func normalizeTaskOutput(output store.TaskOutput) store.TaskOutput {
	if output.Repository != nil {
		repository := output.Repository.Clone()
		repository.ChangedFiles = stringsOrEmpty(repository.ChangedFiles)
		repository.PreExistingFiles = stringsOrEmpty(repository.PreExistingFiles)
		repository.PreservationViolations = stringsOrEmpty(repository.PreservationViolations)
		output.Repository = repository
	}
	if output.Plan != nil {
		plan := *output.Plan
		plan.Steps = stringsOrEmpty(plan.Steps)
		plan.AcceptanceCriteria = stringsOrEmpty(plan.AcceptanceCriteria)
		output.Plan = &plan
	}
	if output.Implementation != nil {
		implementation := *output.Implementation
		implementation.ChangedFiles = stringsOrEmpty(implementation.ChangedFiles)
		output.Implementation = &implementation
	}
	if output.Validation != nil {
		validation := *output.Validation
		if validation.Checks == nil {
			validation.Checks = []store.ValidationEvidence{}
		}
		output.Validation = &validation
	}
	if output.LastReview != nil {
		review := *output.LastReview
		if review.Findings == nil {
			review.Findings = []store.ReviewFinding{}
		}
		review.Suggestions = stringsOrEmpty(review.Suggestions)
		output.LastReview = &review
	}
	return output
}

func stringsOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
