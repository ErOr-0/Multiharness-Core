package structured

import (
	"multiharness-core/internal/store"
	"strings"
	"testing"
)

func TestRoutedQuestionRequiresAnswerOnlyPrompt(t *testing.T) {
	prompt, err := PlanningPrompt(store.TaskInput{Task: "Is this agent loop correct?", WorkingDir: "/workspace", AnswerOnly: true})
	if err != nil || !strings.Contains(prompt, `You MUST return action="answer"`) || !strings.Contains(prompt, "Do not return an implementation plan or perform changes") || strings.Contains(prompt, "You are the planning stage") {
		t.Fatal(prompt, err)
	}
}
