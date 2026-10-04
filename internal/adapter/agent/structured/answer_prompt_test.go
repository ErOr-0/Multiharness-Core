package structured

import (
	"strings"
	"testing"

	"multiharness-core/internal/contract"
)

func TestRoutedQuestionRequiresAnswerOnlyPrompt(t *testing.T) {
	prompt, err := PlanningPromptWithBudget(contract.TaskInput{Task: "Is this agent loop correct?", WorkingDir: "/workspace", AnswerOnly: true}, DefaultBudget())
	if err != nil || !strings.Contains(prompt, `You MUST return action="answer"`) || !strings.Contains(prompt, "Do not return an implementation plan or perform changes") || strings.Contains(prompt, "You are the planning stage") {
		t.Fatal(prompt, err)
	}
}
