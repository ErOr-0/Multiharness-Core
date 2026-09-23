package store

import (
	"strings"
	"unicode/utf8"
)

// ConversationTurn is a completed exchange from the current interactive Team
// conversation. It is explicit context because agent sessions are role-local.
type ConversationTurn struct {
	ID        string `json:"id,omitempty"`
	User      string `json:"user"`
	Assistant string `json:"assistant"`
}

type TaskInput struct {
	// AnswerOnly restricts the read-only agent to an answer, never a change plan.
	AnswerOnly               bool               `json:"answer_only,omitempty"`
	PlanOnly                 bool               `json:"plan_only,omitempty"`
	SelectedPlan             *Plan              `json:"selected_plan,omitempty"`
	SelectedPlanStale        bool               `json:"selected_plan_stale,omitempty"`
	PlanArtifactID           string             `json:"-"`
	CaseArtifactID           string             `json:"-"`
	ImplementationArtifactID string             `json:"-"`
	Task                     string             `json:"task"`
	WorkingDir               string             `json:"working_dir"`
	MaxRepairAttempts        int                `json:"max_repair_attempts"`
	SessionID                string             `json:"session_id,omitempty"`
	RecentTurns              []ConversationTurn `json:"recent_turns,omitempty"`
}

func (input TaskInput) RepairAvailable(completedRepairAttempts int) bool {
	return completedRepairAttempts >= 0 &&
		input.MaxRepairAttempts >= 0 &&
		completedRepairAttempts < input.MaxRepairAttempts
}

func (input TaskInput) Validate() error {
	if strings.TrimSpace(input.Task) == "" {
		return invalid("task", "must not be blank")
	}
	if strings.TrimSpace(input.WorkingDir) == "" {
		return invalid("working_dir", "must not be blank")
	}
	if input.MaxRepairAttempts < 0 {
		return invalid("max_repair_attempts", "must be zero or greater")
	}
	if input.SelectedPlan != nil {
		if err := input.SelectedPlan.Validate(); err != nil {
			return nested("selected_plan", err)
		}
		if (input.SelectedPlan.Action != PlanActionPropose && input.SelectedPlan.Action != PlanActionImplement) || input.SelectedPlan.ID == "" || input.SelectedPlan.Version < 1 {
			return invalid("selected_plan", "requires a persisted plan with ID and version")
		}
	}
	if input.SessionID != "" && (strings.ContainsAny(input.SessionID, " \t\r\n\x00") || strings.HasPrefix(input.SessionID, "-")) {
		return invalid("session_id", "must not contain whitespace, control characters, or leading dashes")
	}
	if len(input.RecentTurns) > 6 {
		return invalid("recent_turns", "must contain at most six completed exchanges")
	}
	bytes := 0
	for _, turn := range input.RecentTurns {
		if strings.TrimSpace(turn.User) == "" || strings.TrimSpace(turn.Assistant) == "" ||
			!utf8.ValidString(turn.User) || !utf8.ValidString(turn.Assistant) ||
			strings.ContainsRune(turn.User, 0) || strings.ContainsRune(turn.Assistant, 0) {
			return invalid("recent_turns", "must contain valid nonempty UTF-8 text")
		}
		bytes += len(turn.User) + len(turn.Assistant)
	}
	if bytes > 24<<10 {
		return invalid("recent_turns", "must be at most 24 KiB")
	}
	return nil
}
