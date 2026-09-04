package codex

import (
	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
)

func buildPlannerPrompt(input store.TaskInput) (string, error) {
	return structured.PlanningPrompt(input)
}
