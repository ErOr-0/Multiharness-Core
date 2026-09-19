package store

// PlanningDecision is the Jev-style router output for planning.
type PlanningDecision struct {
	NeedsPlanning bool               `json:"needs_planning"`
	Confidence    float64            `json:"confidence"`
	Reason        string             `json:"reason"`
	Model         string             `json:"model"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
}

// ReviewDecision is the Jev-style router output for review.
// When ShouldReview is false, Approved indicates the synthesized verdict.
type ReviewDecision struct {
	ShouldReview  bool               `json:"should_review"`
	Approved      bool               `json:"approved"`
	Confidence    float64            `json:"confidence"`
	Reason        string             `json:"reason"`
	Model         string             `json:"model"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
}
