package cli

import (
	"fmt"
	"testing"

	"multiharness-core/internal/contract"
)

func TestRelevantContextIsBoundedAndGreetingsStayCheap(t *testing.T) {
	var turns []contract.ConversationTurn
	for i := 0; i < 6; i++ {
		turns = append(turns, contract.ConversationTurn{ID: fmt.Sprintf("turn_%d", i), User: fmt.Sprintf("request %d", i), Assistant: fmt.Sprintf("answer %d", i)})
	}
	if got := contract.SelectRecentTurns("hello", turns); len(got) != 0 {
		t.Fatalf("greeting included %d turns", len(got))
	}
	if got := contract.SelectRecentTurns("Add another endpoint", turns); len(got) != 6 || got[0].ID != "turn_0" {
		t.Fatalf("ordinary task: %+v", got)
	}
	selected := contract.SelectRecentTurns("continue this work", turns)
	if len(selected) != 6 || selected[0].ID != "turn_0" {
		t.Fatalf("follow-up: %+v", selected)
	}
	if bytes := retrievedContextBytes(contract.TaskInput{RecentTurns: selected}); bytes <= 0 || bytes >= 24<<10 {
		t.Fatalf("retrieved context bytes: %d", bytes)
	}
}
