package structured

import "multiharness-core/internal/contract"

// Wire responses remain adapter-owned so provider schema versioning never
// leaks into workflow contracts.
type planResponse struct {
	Action             *contract.PlanAction `json:"action"`
	Title              *string              `json:"title,omitempty"`
	Tags               *[]string            `json:"tags,omitempty"`
	Answer             *string              `json:"answer"`
	SchemaVersion      *schemaVersion       `json:"schema_version"`
	Summary            *string              `json:"summary"`
	HandoffContext     *[]string            `json:"handoff_context"`
	Steps              *[]string            `json:"steps"`
	AcceptanceCriteria *[]string            `json:"acceptance_criteria"`
}

type reviewResponse struct {
	ValidationAction *contract.ValidationAction `json:"validation_action"`
	SchemaVersion    *schemaVersion             `json:"schema_version"`
	Approved         *bool                      `json:"approved"`
	Summary          *string                    `json:"summary"`
	Findings         *[]reviewFindingResponse   `json:"findings"`
	Suggestions      *[]string                  `json:"suggestions"`
}

type reviewFindingResponse struct {
	Severity       *contract.FindingSeverity `json:"severity"`
	Blocking       *bool                     `json:"blocking"`
	File           *string                   `json:"file"`
	Line           *int                      `json:"line"`
	Description    *string                   `json:"description"`
	Evidence       *string                   `json:"evidence"`
	RequiredAction *string                   `json:"required_action"`
}
