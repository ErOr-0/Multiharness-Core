package store

import "fmt"

// PermissionDenied carries native denial evidence across the adapter boundary.
// It never grants access or authorizes an automatic retry.
type PermissionDenied struct {
	SessionID string        `json:"session_id"`
	Action    BlockedAction `json:"action"`
}

func (p *PermissionDenied) Error() string {
	if p.Action.Target == "" {
		return fmt.Sprintf("permission denied for %q", p.Action.Tool)
	}
	return fmt.Sprintf("permission denied for %q on %q", p.Action.Tool, p.Action.Target)
}

func (p PermissionDenied) Validate() error {
	if p.SessionID == "" {
		return invalid("session_id", "is required for a permission denial")
	}
	return (DirectResponse{SessionID: p.SessionID, NeedsInput: true, Blocked: &p.Action}).Validate()
}
