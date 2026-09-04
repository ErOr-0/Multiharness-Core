package store

import (
	"slices"
	"strings"
)

// RepositoryState identifies an independently captured checkout, including its
// index and non-ignored file contents. An empty Head denotes an unborn branch.
type RepositoryState struct {
	Root        string `json:"root"`
	Head        string `json:"head"`
	Status      string `json:"status"`
	Fingerprint string `json:"fingerprint"`
}

// RepositoryEvidence separates the starting checkout from changes observed
// during this run. Complete=false means inspection failed or was cancelled;
// Current must not then be used as evidence of the latest checkout.
type RepositoryEvidence struct {
	Baseline               RepositoryState `json:"baseline"`
	Current                RepositoryState `json:"current"`
	Complete               bool            `json:"complete"`
	ChangedFiles           []string        `json:"changed_files"`
	PreExistingFiles       []string        `json:"pre_existing_files"`
	PreservationViolations []string        `json:"preservation_violations"`
	Diff                   string          `json:"diff"`
	RecoveryDirectory      string          `json:"recovery_directory,omitempty"`
}

func (evidence RepositoryEvidence) Validate() error {
	if strings.TrimSpace(evidence.Baseline.Root) == "" || evidence.Baseline.Fingerprint == "" {
		return invalid("baseline", "root and fingerprint are required")
	}
	if evidence.Complete && (evidence.Current.Root != evidence.Baseline.Root || evidence.Current.Fingerprint == "") {
		return invalid("current", "complete evidence requires the same root and a fingerprint")
	}
	if err := validateStrings("changed_files", evidence.ChangedFiles, false); err != nil {
		return err
	}
	if err := validateStrings("pre_existing_files", evidence.PreExistingFiles, false); err != nil {
		return err
	}
	return validateStrings("preservation_violations", evidence.PreservationViolations, false)
}

func validateRepository(evidence *RepositoryEvidence) error {
	if evidence == nil {
		return nil
	} // Isolated agent adapters can be tested without a workspace.
	if err := evidence.Validate(); err != nil {
		return nested("repository", err)
	}
	return nil
}

// Clone prevents consumers from mutating the workflow's recorded evidence.
func (evidence RepositoryEvidence) Clone() *RepositoryEvidence {
	evidence.ChangedFiles = slices.Clone(evidence.ChangedFiles)
	evidence.PreExistingFiles = slices.Clone(evidence.PreExistingFiles)
	evidence.PreservationViolations = slices.Clone(evidence.PreservationViolations)
	return &evidence
}
