package store

import "strings"

// ImplementationRequest contains everything needed for initial execution.
type ImplementationRequest struct {
	Input      TaskInput           `json:"input"`
	Plan       Plan                `json:"plan"`
	Repository *RepositoryEvidence `json:"repository,omitempty"`
}

// Validate checks an initial implementation request.
func (request ImplementationRequest) Validate() error {
	if err := request.Input.Validate(); err != nil {
		return nested("input", err)
	}
	if err := request.Plan.ValidateImplementation(); err != nil {
		return nested("plan", err)
	}
	return validateRepository(request.Repository)
}

// ImplementationResult describes the implementer's work. Repository-derived
// changes can replace or reconcile ChangedFiles at the application boundary.
type ImplementationResult struct {
	Summary        string   `json:"summary"`
	ChangedFiles   []string `json:"changed_files"`
	AgentSessionID string   `json:"agent_session_id,omitempty"`
}

// Validate checks the implementer's result without assuming that it also ran
// independent validation.
func (result ImplementationResult) Validate() error {
	if strings.TrimSpace(result.Summary) == "" {
		return invalid("summary", "must not be blank")
	}
	return validateStrings("changed_files", result.ChangedFiles, false)
}
