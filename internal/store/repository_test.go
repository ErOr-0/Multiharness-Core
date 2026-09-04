package store

import "testing"

func validRepositoryEvidence() *RepositoryEvidence {
	state := RepositoryState{Root: "/workspace/project", Fingerprint: "fingerprint"}
	return &RepositoryEvidence{Baseline: state, Current: state, Complete: true, ChangedFiles: []string{"server.go", "server_test.go"}, PreExistingFiles: []string{"notes.txt"}, PreservationViolations: []string{}}
}

func TestRepositoryEvidenceContracts(t *testing.T) {
	evidence := validRepositoryEvidence()
	assertValidationResult(t, evidence.Validate(), "")
	assertJSONRoundTrip(t, *evidence)
	copy := evidence.Clone()
	copy.ChangedFiles[0] = "mutated"
	if evidence.ChangedFiles[0] != "server.go" {
		t.Fatal("Clone aliased changed files")
	}
	copy.Current.Root = "different"
	assertValidationResult(t, copy.Validate(), "current")
	copy.Complete = false
	assertValidationResult(t, copy.Validate(), "")
}

func TestCompletedOutputRequiresIndependentRepositoryEvidence(t *testing.T) {
	plan, implementation, validation, review := validPlan(), validImplementationResult(), validValidationReport(), validApprovedReview()
	output := TaskOutput{Status: TaskStatusApproved, Summary: "approved", Plan: &plan, Implementation: &implementation, Validation: &validation, LastReview: &review}
	assertValidationResult(t, output.Validate(), "repository")
	output.Repository = validRepositoryEvidence()
	output.Repository.PreservationViolations = []string{"notes.txt"}
	assertValidationResult(t, output.Validate(), "repository.preservation_violations")
	output.Repository.PreservationViolations = nil
	output.Repository.ChangedFiles = []string{"invented.go"}
	assertValidationResult(t, output.Validate(), "implementation.changed_files")
}

func TestRepositoryEvidenceSurvivesAllCrossAgentContracts(t *testing.T) {
	repository := validRepositoryEvidence()
	input, plan, implementation, validation, review := validTaskInput(), validPlan(), validImplementationResult(), validValidationReport(), validRejectedReview()
	assertJSONRoundTrip(t, ImplementationRequest{Input: input, Plan: plan, Repository: repository})
	assertJSONRoundTrip(t, ValidationRequest{Input: input, Plan: plan, Implementation: implementation, Repository: repository})
	assertJSONRoundTrip(t, ReviewRequest{Input: input, Plan: plan, Implementation: implementation, Validation: validation, Repository: repository})
	assertJSONRoundTrip(t, RepairRequest{Input: input, Plan: plan, Implementation: implementation, Validation: validation, Review: review, Repository: repository})
}
