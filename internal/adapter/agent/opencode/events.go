package opencode

// ProgressType identifies a stable, useful subset of OpenCode's JSON events.
type ProgressType string

const (
	ProgressStepStarted  ProgressType = "step_started"
	ProgressToolFinished ProgressType = "tool_finished"
	ProgressStepFinished ProgressType = "step_finished"
)

// ProgressEvent is intentionally smaller than OpenCode's provider event. This
// keeps callers independent of OpenCode's evolving wire format.
type ProgressEvent struct {
	Type      ProgressType `json:"type"`
	SessionID string       `json:"session_id"`
	Tool      string       `json:"tool,omitempty"`
	Status    string       `json:"status,omitempty"`
}

// ProgressSink receives synchronous implementation progress. Implementations
// should return quickly and be safe for the concurrency in which they are used.
type ProgressSink interface {
	Publish(event ProgressEvent)
}

type discardProgressSink struct{}

func (discardProgressSink) Publish(ProgressEvent) {}
