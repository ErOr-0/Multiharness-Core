package store

import "math"

// TaskRoute is the explicit intent selected before a Team agent starts.
type TaskRoute string

const (
	RouteAnswer    TaskRoute = "answer"
	RoutePlan      TaskRoute = "needs_planning"
	RouteImplement TaskRoute = "direct_implement"
)

func (r TaskRoute) Valid() bool { return r == RouteAnswer || r == RoutePlan || r == RouteImplement }

type DecisionSource string

const (
	DecisionJev      DecisionSource = "jev"
	DecisionFallback DecisionSource = "fallback"
)

type RoutingFallback string

const (
	RoutingUnavailable   RoutingFallback = "unavailable"
	RoutingInvalid       RoutingFallback = "invalid_response"
	RoutingLowConfidence RoutingFallback = "low_confidence"
)

func (r RoutingFallback) Valid() bool {
	return r == RoutingUnavailable || r == RoutingInvalid || r == RoutingLowConfidence
}

// PlanningDecision is the Jev-style router output for planning.
type PlanningDecision struct {
	Route         TaskRoute          `json:"route"`
	Source        DecisionSource     `json:"source"`
	Fallback      RoutingFallback    `json:"fallback,omitempty"`
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

func (d PlanningDecision) Validate() error {
	if !d.Route.Valid() || (d.Source != DecisionJev && d.Source != DecisionFallback) {
		return invalid("routing", "invalid route or source")
	}
	if math.IsNaN(d.Confidence) || math.IsInf(d.Confidence, 0) || d.Confidence < 0 || d.Confidence > 1 {
		return invalid("routing", "invalid confidence")
	}
	if d.NeedsPlanning != (d.Route == RoutePlan) {
		return invalid("routing", "inconsistent planning flag")
	}
	if d.Source == DecisionFallback {
		if !d.Fallback.Valid() || d.Route != RoutePlan {
			return invalid("routing", "fallback must preserve read-only assessment")
		}
	} else if d.Fallback != "" {
		return invalid("routing", "Jev decision cannot claim fallback")
	}
	return nil
}
