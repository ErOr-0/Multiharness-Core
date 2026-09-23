package cli

import (
	"fmt"
	"testing"

	"multiharness-core/internal/store"
)

func TestRelevantContextIsBoundedAndGreetingsStayCheap(t *testing.T) {
	var turns []store.ConversationTurn
	for i := 0; i < 6; i++ {
		turns = append(turns, store.ConversationTurn{ID: fmt.Sprintf("turn_%d", i), User: fmt.Sprintf("request %d", i), Assistant: fmt.Sprintf("answer %d", i)})
	}
	if got := selectRecentTurns("hello", turns); len(got) != 0 {
		t.Fatalf("greeting included %d turns", len(got))
	}
	if got := selectRecentTurns("Add another endpoint", turns); len(got) != 1 || got[0].ID != "turn_5" {
		t.Fatalf("ordinary task: %+v", got)
	}
	selected := selectRecentTurns("continue this work", turns)
	if len(selected) != 3 || selected[0].ID != "turn_3" {
		t.Fatalf("follow-up: %+v", selected)
	}
	if bytes := retrievedContextBytes(store.TaskInput{RecentTurns: selected}); bytes <= 0 || bytes >= 24<<10 {
		t.Fatalf("retrieved context bytes: %d", bytes)
	}
}
