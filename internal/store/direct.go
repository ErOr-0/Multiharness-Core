package store

import "strings"

// DirectResponse is native agent output, not independent validation evidence.
type DirectResponse struct {
	Text       string         `json:"text,omitempty"`
	SessionID  string         `json:"session_id,omitempty"`
	NeedsInput bool           `json:"needs_input,omitempty"`
	Blocked    *BlockedAction `json:"blocked_action,omitempty"`
}

// BlockedAction describes a native permission denial, not an approval granted
// by Multiharness. The next turn keeps the provider's existing permission policy.
type BlockedAction struct {
	Tool   string `json:"tool"`
	Target string `json:"target,omitempty"`
}

func (r DirectResponse) Validate() error {
	if r.Blocked != nil && (!r.NeedsInput || strings.TrimSpace(r.Blocked.Tool) == "" || len(r.Blocked.Tool) > 128 || len(r.Blocked.Target) > 2048) {
		return invalid("blocked_action", "requires input and a bounded native tool description")
	}
	if strings.ContainsAny(r.SessionID, " \t\r\n\x00") || strings.HasPrefix(r.SessionID, "-") {
		return invalid("session_id", "must be a CLI session identifier")
	}
	return nil
}
