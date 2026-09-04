package store

import (
	"fmt"
	"strings"
)

// FindingSeverity communicates impact independently from whether a finding
// blocks approval.
type FindingSeverity string

const (
	FindingSeverityInfo     FindingSeverity = "info"
	FindingSeverityWarning  FindingSeverity = "warning"
	FindingSeverityError    FindingSeverity = "error"
	FindingSeverityCritical FindingSeverity = "critical"
)

// ReviewFinding is one actionable, evidence-backed reviewer observation.
// Line is one-based when present and zero when no precise line applies.
type ReviewFinding struct {
	Severity       FindingSeverity `json:"severity"`
	Blocking       bool            `json:"blocking"`
	File           string          `json:"file,omitempty"`
	Line           int             `json:"line,omitempty"`
	Description    string          `json:"description"`
	Evidence       string          `json:"evidence,omitempty"`
	RequiredAction string          `json:"required_action,omitempty"`
}

// Validate checks that a finding is structured and actionable when blocking.
func (finding ReviewFinding) Validate() error {
	if !finding.Severity.valid() {
		return invalid("severity", fmt.Sprintf("unsupported value %q", finding.Severity))
	}
	if finding.Line < 0 {
		return invalid("line", "must be zero or greater")
	}
	if finding.Line > 0 && strings.TrimSpace(finding.File) == "" {
		return invalid("file", "is required when line is set")
	}
	if strings.TrimSpace(finding.Description) == "" {
		return invalid("description", "must not be blank")
	}
	if finding.Blocking && strings.TrimSpace(finding.RequiredAction) == "" {
		return invalid("required_action", "is required for a blocking finding")
	}
	return nil
}

func (severity FindingSeverity) valid() bool {
	switch severity {
	case FindingSeverityInfo, FindingSeverityWarning, FindingSeverityError, FindingSeverityCritical:
		return true
	default:
		return false
	}
}

// Review is the reviewer's structured decision. An approved review cannot
// contain blocking findings; a rejected review must contain at least one.
type Review struct {
	Approved    bool            `json:"approved"`
	Summary     string          `json:"summary"`
	Findings    []ReviewFinding `json:"findings"`
	Suggestions []string        `json:"suggestions"`
}

// Validate checks the review decision and its findings for consistency.
func (review Review) Validate() error {
	if strings.TrimSpace(review.Summary) == "" {
		return invalid("summary", "must not be blank")
	}
	if err := validateStrings("suggestions", review.Suggestions, false); err != nil {
		return err
	}

	blocking := 0
	for i, finding := range review.Findings {
		if err := finding.Validate(); err != nil {
			return nested(fmt.Sprintf("findings[%d]", i), err)
		}
		if finding.Blocking {
			blocking++
		}
	}

	if review.Approved && blocking > 0 {
		return invalid("approved", "cannot be true when blocking findings exist")
	}
	if !review.Approved && blocking == 0 {
		return invalid("findings", "must contain a blocking finding when approval is denied")
	}
	return nil
}

// ReviewRequest keeps implementation and deterministic validation evidence
// separate while providing the reviewer with one cohesive contract.
type ReviewRequest struct {
	Repository     *RepositoryEvidence  `json:"repository,omitempty"`
	Input          TaskInput            `json:"input"`
	Plan           Plan                 `json:"plan"`
	Implementation ImplementationResult `json:"implementation"`
	Validation     ValidationReport     `json:"validation"`
}

// Validate checks the complete evidence supplied to a reviewer.
func (request ReviewRequest) Validate() error {
	if err := request.Input.Validate(); err != nil {
		return nested("input", err)
	}
	if err := request.Plan.ValidateImplementation(); err != nil {
		return nested("plan", err)
	}
	if err := request.Implementation.Validate(); err != nil {
		return nested("implementation", err)
	}
	if err := request.Validation.Validate(); err != nil {
		return nested("validation", err)
	}
	return validateRepository(request.Repository)
}

// RepairRequest contains the complete evidence behind a requested repair.
type RepairRequest struct {
	Repository     *RepositoryEvidence  `json:"repository,omitempty"`
	Input          TaskInput            `json:"input"`
	Plan           Plan                 `json:"plan"`
	Implementation ImplementationResult `json:"implementation"`
	Validation     ValidationReport     `json:"validation"`
	Review         Review               `json:"review"`
}

// Validate checks that a repair request contains a rejected review with at
// least one blocking, actionable finding.
func (request RepairRequest) Validate() error {
	reviewRequest := ReviewRequest{
		Repository:     request.Repository,
		Input:          request.Input,
		Plan:           request.Plan,
		Implementation: request.Implementation,
		Validation:     request.Validation,
	}
	if err := reviewRequest.Validate(); err != nil {
		return err
	}
	if err := request.Review.Validate(); err != nil {
		return nested("review", err)
	}
	if request.Review.Approved {
		return invalid("review.approved", "must be false for a repair request")
	}
	return nil
}
