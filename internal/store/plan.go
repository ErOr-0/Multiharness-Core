package store

import (
	"fmt"
	"strings"
)

type PlanAction string

const (
	PlanActionImplement PlanAction = "implement"
	PlanActionAnswer    PlanAction = "answer"
	PlanActionPropose   PlanAction = "propose"
)

type Plan struct {
	ID                 string     `json:"id,omitempty"`
	CaseID             string     `json:"case_id,omitempty"`
	Version            int        `json:"version,omitempty"`
	Action             PlanAction `json:"action"`
	Title              string     `json:"title,omitempty"`
	Tags               []string   `json:"tags,omitempty"`
	Summary            string     `json:"summary"`
	Answer             string     `json:"answer,omitempty"`
	HandoffContext     []string   `json:"handoff_context,omitempty"`
	Steps              []string   `json:"steps"`
	AcceptanceCriteria []string   `json:"acceptance_criteria"`
}

func (plan Plan) Validate() error {
	if strings.TrimSpace(plan.Summary) == "" {
		return invalid("summary", "must not be blank")
	}
	if len(plan.Title) > 160 || len(plan.Summary) > 2048 || len(plan.Tags) > 3 || len(plan.HandoffContext) > 24 || len(plan.Steps) > 24 || len(plan.AcceptanceCriteria) > 24 {
		return invalid("plan", "metadata or sections exceed the bounded handoff limits")
	}
	bytes := len(plan.Title) + len(plan.Summary)
	for _, sections := range [][]string{plan.Tags, plan.HandoffContext, plan.Steps, plan.AcceptanceCriteria} {
		for _, section := range sections {
			bytes += len(section)
		}
	}
	if bytes > 24<<10 {
		return invalid("plan", "handoff content exceeds 24 KiB")
	}

	switch plan.Action {

	case PlanActionAnswer:
		if strings.TrimSpace(plan.Answer) == "" {
			return invalid("answer", "must not be blank for an answer-only plan")
		}

		if plan.Title != "" || len(plan.Tags) != 0 || len(plan.HandoffContext) != 0 || len(plan.Steps) != 0 || len(plan.AcceptanceCriteria) != 0 {
			return invalid("action", "an answer-only plan cannot contain implementation handoff, steps, or acceptance criteria")
		}
		return nil

	case PlanActionPropose:
		if plan.Answer != "" {
			return invalid("answer", "must be empty for a saved proposal")
		}
		if strings.TrimSpace(plan.Title) == "" {
			return invalid("title", "is required for a saved proposal")
		}
		if err := validateStrings("tags", plan.Tags, true); err != nil {
			return err
		}
		if err := validateStrings("handoff_context", plan.HandoffContext, false); err != nil {
			return err
		}
		if err := validateStrings("steps", plan.Steps, true); err != nil {
			return err
		}
		return validateStrings("acceptance_criteria", plan.AcceptanceCriteria, true)

	case PlanActionImplement:
		if plan.Answer != "" {
			return invalid("answer", "must be empty for an implementation plan")
		}
		if err := validateStrings("tags", plan.Tags, false); err != nil {
			return err
		}

	default:
		return invalid("action", "must be implement, answer, or propose")
	}

	if err := validateStrings("steps", plan.Steps, true); err != nil {
		return err
	}
	if err := validateStrings("handoff_context", plan.HandoffContext, false); err != nil {
		return err
	}

	return validateStrings("acceptance_criteria", plan.AcceptanceCriteria, true)
}

func (plan Plan) ValidateImplementation() error {

	if err := plan.Validate(); err != nil {
		return err
	}

	if plan.Action != PlanActionImplement {
		return invalid("action", "an implementation plan is required")
	}

	return nil
}

// Display presents a saved proposal without asking another model to reformat it.
func (plan Plan) Display() string {
	if plan.Action != PlanActionPropose {
		return plan.Answer
	}
	var body strings.Builder
	fmt.Fprintf(&body, "%s\n\n%s", plan.Title, plan.Summary)
	if len(plan.Tags) > 0 {
		fmt.Fprintf(&body, "\n\nTags: %s", strings.Join(plan.Tags, ", "))
	}
	if len(plan.HandoffContext) > 0 {
		body.WriteString("\n\nContext and constraints:")
		for _, note := range plan.HandoffContext {
			fmt.Fprintf(&body, "\n- %s", note)
		}
	}
	body.WriteString("\n\nSteps:")
	for i, step := range plan.Steps {
		fmt.Fprintf(&body, "\n%d. %s", i+1, step)
	}
	body.WriteString("\n\nAcceptance criteria:")
	for _, criterion := range plan.AcceptanceCriteria {
		fmt.Fprintf(&body, "\n- %s", criterion)
	}
	return body.String()
}
