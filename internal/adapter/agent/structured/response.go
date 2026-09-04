package structured

import "multiharness-core/internal/store"

// Wire responses remain adapter-owned so provider schema versioning never
// leaks into workflow contracts.
type planResponse struct {
	Action             *store.PlanAction `json:"action"`
	Answer             *string           `json:"answer"`
	SchemaVersion      *string           `json:"schema_version"`
	Summary            *string           `json:"summary"`
	Steps              *[]string         `json:"steps"`
	AcceptanceCriteria *[]string         `json:"acceptance_criteria"`
}

type reviewResponse struct {
	SchemaVersion *string                  `json:"schema_version"`
	Approved      *bool                    `json:"approved"`
	Summary       *string                  `json:"summary"`
	Findings      *[]reviewFindingResponse `json:"findings"`
	Suggestions   *[]string                `json:"suggestions"`
}

type reviewFindingResponse struct {
	Severity       *store.FindingSeverity `json:"severity"`
	Blocking       *bool                  `json:"blocking"`
	File           *string                `json:"file"`
	Line           *int                   `json:"line"`
	Description    *string                `json:"description"`
	Evidence       *string                `json:"evidence"`
	RequiredAction *string                `json:"required_action"`
}
