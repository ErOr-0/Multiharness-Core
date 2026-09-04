package opencode

import (
	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
)

func buildImplementationPrompt(request store.ImplementationRequest) (string, error) {
	return structured.ImplementationPrompt(request)
}
func buildRepairPrompt(request store.RepairRequest) (string, error) {
	return structured.RepairPrompt(request)
}
