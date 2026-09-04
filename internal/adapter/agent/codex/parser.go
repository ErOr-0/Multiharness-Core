package codex

import (
	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
)

func parsePlan(data []byte) (store.Plan, error) {
	v, e := structured.ParsePlan(data)
	if e != nil {
		return v, &OutputError{Role: rolePlanning, Cause: e}
	}
	return v, nil
}
func parseReview(data []byte) (store.Review, error) {
	v, e := structured.ParseReview(data)
	if e != nil {
		return v, &OutputError{Role: roleReview, Cause: e}
	}
	return v, nil
}
