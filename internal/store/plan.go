package store

import "strings"

type PlanAction string

const (
	PlanActionImplement PlanAction = "implement"
	PlanActionAnswer    PlanAction = "answer"
)

// Plan is the planner's structured contract with the implementer and reviewer.
type Plan struct {
	Action             PlanAction `json:"action"`
	Summary            string     `json:"summary"`
	Answer             string     `json:"answer,omitempty"`
	Steps              []string   `json:"steps"`
	AcceptanceCriteria []string   `json:"acceptance_criteria"`
}

// Validate checks the planner's structured output.
func (plan Plan) Validate() error {
	if strings.TrimSpace(plan.Summary) == "" {
		return invalid("summary", "must not be blank")
	}
	switch plan.Action {
	case PlanActionAnswer:
		if strings.TrimSpace(plan.Answer) == "" {
			return invalid("answer", "must not be blank for an answer-only plan")
		}
		if len(plan.Steps) != 0 || len(plan.AcceptanceCriteria) != 0 {
			return invalid("action", "an answer-only plan cannot contain implementation steps or acceptance criteria")
		}
		return nil
	case PlanActionImplement:
		if plan.Answer != "" {
			return invalid("answer", "must be empty for an implementation plan")
		}
	default:
		return invalid("action", "must be implement or answer")
	}
	if err := validateStrings("steps", plan.Steps, true); err != nil {
		return err
	}
	return validateStrings("acceptance_criteria", plan.AcceptanceCriteria, true)
}

// ValidateImplementation prevents answer-only plans from crossing coding ports.
func (plan Plan) ValidateImplementation() error {
	if err := plan.Validate(); err != nil {
		return err
	}
	if plan.Action != PlanActionImplement {
		return invalid("action", "an implementation plan is required")
	}
	return nil
}
