package store

import "strings"

type ImplementationRequest struct {
	Input      TaskInput           `json:"input"`
	Plan       Plan                `json:"plan"`
	Repository *RepositoryEvidence `json:"repository,omitempty"`
}

func (request ImplementationRequest) Validate() error {

	if err := request.Input.Validate(); err != nil {
		return nested("input", err)
	}

	if err := request.Plan.ValidateImplementation(); err != nil {
		return nested("plan", err)
	}

	return validateRepository(request.Repository)
}

type ImplementationResult struct {
	ID             string   `json:"id,omitempty"`
	Version        int      `json:"version,omitempty"`
	Summary        string   `json:"summary"`
	ChangedFiles   []string `json:"changed_files"`
	AgentSessionID string   `json:"agent_session_id,omitempty"`
}

func (result ImplementationResult) Validate() error {
	if strings.TrimSpace(result.Summary) == "" {
		return invalid("summary", "must not be blank")
	}
	if len(result.Summary) > MaxImplementationSummaryBytes {
		return invalid("summary", "exceeds the bounded handoff limit")
	}
	if len(result.ChangedFiles) > MaxChangedFiles {
		return invalid("changed_files", "exceeds the bounded handoff limit")
	}
	for _, name := range result.ChangedFiles {
		if len(name) > MaxChangedFilePathBytes {
			return invalid("changed_files", "entry exceeds the bounded handoff limit")
		}
	}
	return validateStrings("changed_files", result.ChangedFiles, false)
}
