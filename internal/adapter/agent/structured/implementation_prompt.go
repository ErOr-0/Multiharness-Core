package structured

import (
	"encoding/json"
	"fmt"

	"multiharness-core/internal/store"
)

const finalResponseInstructions = `

When the work is complete, your final response must be only one JSON object with exactly this shape (no Markdown fence or commentary):
{"schema_version":"1","summary":"concise factual summary","changed_files":["relative/path"]}
Use an empty changed_files array when no files changed. Do not include an agent session ID; Multiharness captures it independently.`

func ImplementationPrompt(request store.ImplementationRequest) (string, error) {
	payload, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode implementation request: %w", err)
	}

	return `You are the implementation stage of Multiharness.

Implement the supplied plan in the target repository. Inspect the repository before editing, follow its local instructions and conventions, preserve unrelated existing changes, and keep the change focused on the task. Run relevant focused checks when practical. Do not create commits, push changes, or claim success for work you did not complete.

Files listed in repository.pre_existing_files are protected: do not edit, delete, or rename them. Do not stage files or change Git HEAD.

Implementation request:
` + string(payload) + finalResponseInstructions, nil
}

type repairPromptPayload struct {
	Repository       *store.RepositoryEvidence  `json:"repository,omitempty"`
	Input            store.TaskInput            `json:"input"`
	Plan             store.Plan                 `json:"plan"`
	Implementation   store.ImplementationResult `json:"implementation"`
	Validation       store.ValidationReport     `json:"validation"`
	ReviewSummary    string                     `json:"review_summary"`
	BlockingFindings []store.ReviewFinding      `json:"blocking_findings"`
}

func RepairPrompt(request store.RepairRequest) (string, error) {
	blocking := make([]store.ReviewFinding, 0, len(request.Review.Findings))
	for _, finding := range request.Review.Findings {
		if finding.Blocking {
			blocking = append(blocking, finding)
		}
	}
	payload, err := json.MarshalIndent(repairPromptPayload{
		Repository:       request.Repository,
		Input:            request.Input,
		Plan:             request.Plan,
		Implementation:   request.Implementation,
		Validation:       request.Validation,
		ReviewSummary:    request.Review.Summary,
		BlockingFindings: blocking,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode repair request: %w", err)
	}

	return `You are the repair stage of Multiharness, continuing an implementation that received a blocking independent review.

Fix every supplied blocking finding while preserving correct existing work and unrelated user changes. Use the original task and plan as the source of intent, and use the latest validation report and concrete review evidence to guide the repair. Inspect the current repository state rather than relying only on the earlier implementation summary. Run relevant focused checks when practical. Do not create commits, push changes, or claim success for work you did not complete.

Files listed in repository.pre_existing_files remain protected. Do not stage files or change Git HEAD.

Repair request:
` + string(payload) + finalResponseInstructions, nil
}
