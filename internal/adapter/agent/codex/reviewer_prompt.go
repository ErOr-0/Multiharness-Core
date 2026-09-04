package codex

import (
	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
)

func buildReviewerPrompt(request store.ReviewRequest) (string, error) {
	return structured.ReviewPrompt(request)
}
