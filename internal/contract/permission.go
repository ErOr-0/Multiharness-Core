package contract

import "fmt"

// PermissionDenied carries native denial evidence across the adapter boundary.
// It never grants access or authorizes an automatic retry.
type PermissionDenied struct {
	SessionID string        `json:"session_id,omitempty"`
	Action    BlockedAction `json:"action"`
	// A decision already made in the native approval dialog must not trigger a
	// second permission-recovery prompt or replay the declined operation.
	UserDeclined bool `json:"user_declined,omitempty"`
}

func (p *PermissionDenied) Error() string {
	if p.Action.Target == "" {
		return fmt.Sprintf("permission denied for %q", p.Action.Tool)
	}
	return fmt.Sprintf("permission denied for %q on %q", p.Action.Tool, p.Action.Target)
}

func (p PermissionDenied) Validate() error {
	// Fresh/ephemeral providers may report a denial without a resumable session.
	return (DirectResponse{SessionID: p.SessionID, NeedsInput: true, Blocked: &p.Action}).Validate()
}
