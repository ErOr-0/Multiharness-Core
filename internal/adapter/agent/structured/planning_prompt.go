package structured

import (
	"encoding/json"
	"fmt"

	"multiharness-core/internal/store"
)

func PlanningPrompt(input store.TaskInput) (string, error) {
	payload, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode planning request: %w", err)
	}

	return `You are the planning stage of Multiharness.

Work in planning mode only. Inspect the target repository with read-only commands as needed. Do not edit files, create commits, or run commands that mutate the repository.

First decide whether the user requested repository changes. For explanations, questions, or reviews that do not authorize changes, use action="answer", provide the complete response in answer, and leave steps and acceptance_criteria empty. Do not send a question-only request to the implementation agent. Ask for missing information in the answer if the task cannot safely be planned yet.

For requested repository changes, use action="implement", leave answer empty, and produce a precise implementation plan grounded in the repository's actual architecture. Keep responsibilities cohesive, respect SOLID and existing project conventions, and include deterministic acceptance criteria. Always include a brief summary. Your final response must be only one JSON object conforming exactly to the supplied version-2 output schema.

Planning request:
` + string(payload), nil
}
