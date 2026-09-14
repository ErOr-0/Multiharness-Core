package store

import "strings"

// DirectResponse is native agent output, not independent validation evidence.
type DirectResponse struct {
	Text       string `json:"text,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	NeedsInput bool   `json:"needs_input,omitempty"`
}

func (r DirectResponse) Validate() error {
	if strings.ContainsAny(r.SessionID, " \t\r\n\x00") || strings.HasPrefix(r.SessionID, "-") {
		return invalid("session_id", "must be a CLI session identifier")
	}
	return nil
}
