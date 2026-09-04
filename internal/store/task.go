package store

import "strings"

// TaskInput is the user-controlled input to one workflow run.
// MaxRepairAttempts counts repair invocations, not reviews. The initial
// implementation is always eligible for one review, so a value of N permits
// at most N repairs and N+1 reviews.
type TaskInput struct {
	Task              string `json:"task"`
	WorkingDir        string `json:"working_dir"`
	MaxRepairAttempts int    `json:"max_repair_attempts"`
	SessionID         string `json:"session_id,omitempty"`
}

// RepairAvailable reports whether another repair may be attempted after the
// supplied number of completed repair attempts.
func (input TaskInput) RepairAvailable(completedRepairAttempts int) bool {
	return completedRepairAttempts >= 0 &&
		input.MaxRepairAttempts >= 0 &&
		completedRepairAttempts < input.MaxRepairAttempts
}

// Validate performs pure structural checks. Workspace availability and
// permissions are checked through the workflow Workspace port.
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
	if input.SessionID != "" && (strings.ContainsAny(input.SessionID, " \t\r\n\x00") || strings.HasPrefix(input.SessionID, "-")) {
		return invalid("session_id", "must not contain whitespace, control characters, or leading dashes")
	}
	return nil
}
