package structured

import (
	"encoding/json"
	"fmt"

	"multiharness-core/internal/store"
)

func ReviewPrompt(request store.ReviewRequest) (string, error) {
	payload, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode review request: %w", err)
	}

	return `You are the independent review stage of Multiharness.

Review only. Do not edit files, create commits, or run commands that mutate the repository. This is a fresh reviewer invocation; do not rely on an implementation-agent session or its memory.

The selected workspace is the user-chosen folder and may contain multiple projects. Git is not required. All evidence paths are relative to the selected workspace and include project folder prefixes. Independently inspect relevant projects and the diff before deciding:
- Inspect relevant source files and the supplied baseline-relative file changes. Do not require repository status, commits, staging, or Git commands. Do not initialize Git.
- Treat implementation.changed_files and implementation.summary as claims to verify, not trusted repository evidence.
- When repository evidence is supplied, repository.diff and repository.changed_files describe changes since the captured baseline, not all differences against HEAD. Distinguish pre_existing_files from this run's changes. Never approve incomplete evidence or preservation violations. Cross-check the captured current state against the live checkout.
- Evaluate the original task, approved plan, observed repository state, observed diff, and deterministic validation evidence together.
- Do not approve when deterministic validation failed or when a task/acceptance requirement has a blocking defect.
- Make every blocking finding concrete, evidence-backed, and actionable.

The request below supplies the task, plan, implementation claim, and independently produced validation evidence. Your final response must be only one JSON object conforming exactly to the supplied output schema.

Review request:
` + string(payload) + commandEvidenceInstructions, nil
}
