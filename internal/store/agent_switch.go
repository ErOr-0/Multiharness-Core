package store

import "strings"

// AgentSwitch records explicit consent to move one role to an alternate agent
// for the remainder of this run. Implementation includes later repair calls.
type AgentSwitch struct {
	Stage    WorkflowStage `json:"stage"`
	From     string        `json:"from"`
	To       string        `json:"to"`
	Model    string        `json:"model"`
	CanWrite bool          `json:"can_write"`
}

func (s AgentSwitch) Validate() error {
	if s.Stage != WorkflowStagePlanning && s.Stage != WorkflowStageReview && s.Stage != WorkflowStageImplementation && s.Stage != WorkflowStageRepair {
		return invalid("stage", "unsupported agent switch stage")
	}
	if s.From == s.To || strings.TrimSpace(s.Model) == "" || strings.ContainsAny(s.Model, "\r\n\x00") {
		return invalid("agent_switch", "distinct agents and a model description are required")
	}
	for _, name := range []string{s.From, s.To} {
		if name == "" {
			return invalid("agent", "name is required")
		}
		for _, c := range name {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return invalid("agent", "name must be an identifier")
			}
		}
	}
	if s.CanWrite != (s.Stage == WorkflowStageImplementation || s.Stage == WorkflowStageRepair) {
		return invalid("can_write", "must match the requested role")
	}
	return nil
}
