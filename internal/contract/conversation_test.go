package contract_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"multiharness-core/internal/contract"
)

func TestAppendTurnKeepsABoundedValidWindow(t *testing.T) {
	var turns []contract.ConversationTurn
	long := strings.Repeat("é", contract.MaxTurnTextBytes)
	for i := 0; i < 10; i++ {
		turns = contract.AppendTurn(turns, fmt.Sprintf("request %d \x1b[31m%s", i, long), contract.TaskOutput{Status: contract.TaskStatusResponded, Summary: long})
	}
	if len(turns) == 0 || len(turns) > contract.MaxRecentTurns || contract.ConversationBytes(turns) > contract.MaxRecentTurnBytes {
		t.Fatalf("window: %d turns, %d bytes", len(turns), contract.ConversationBytes(turns))
	}
	last := turns[len(turns)-1]
	if !strings.HasPrefix(last.User, "request 9 [31m") || !utf8.ValidString(last.User) || len(last.User) > contract.MaxTurnTextBytes || !strings.HasSuffix(last.User, "[remaining text truncated]") {
		t.Fatalf("turn text not sanitised and bounded: %d bytes %q", len(last.User), last.User[:40])
	}
	input := contract.TaskInput{Task: "next", WorkingDir: "/workspace", RecentTurns: turns}
	if err := input.Validate(); err != nil {
		t.Fatalf("a built window must validate: %v", err)
	}
}

func TestAppendTurnSkipsCancelledAndEmptyExchanges(t *testing.T) {
	turns := contract.AppendTurn(nil, "task", contract.TaskOutput{Status: contract.TaskStatusCancelled, Summary: "stopped"})
	turns = contract.AppendTurn(turns, "task", contract.TaskOutput{Status: contract.TaskStatusResponded, Summary: "  "})
	if len(turns) != 0 {
		t.Fatalf("recorded %+v", turns)
	}
	turns = contract.AppendTurn(turns, "task", contract.TaskOutput{Status: contract.TaskStatusFailed, Summary: "Agent failed", Failure: &contract.TaskFailure{Message: "quota"}})
	if len(turns) != 1 || turns[0].Assistant != "Agent failed\nquota" {
		t.Fatalf("failure reason lost: %+v", turns)
	}
}

func TestSelectRecentTurnsCopiesTheNewestWindow(t *testing.T) {
	var turns []contract.ConversationTurn
	for i := 0; i < 8; i++ {
		turns = append(turns, contract.ConversationTurn{ID: fmt.Sprintf("turn_%d", i), User: "u", Assistant: "a"})
	}
	if got := contract.SelectRecentTurns(" Hello! ", turns); got != nil {
		t.Fatalf("greeting included %d turns", len(got))
	}
	got := contract.SelectRecentTurns("Add another endpoint", turns)
	if len(got) != contract.MaxRecentTurns || got[0].ID != "turn_2" {
		t.Fatalf("window: %+v", got)
	}
	got[0].ID = "changed"
	if turns[2].ID != "turn_2" {
		t.Fatal("selection aliases the caller's turns")
	}
	if got := contract.SelectRecentTurns("task", nil); len(got) != 0 {
		t.Fatalf("empty history: %+v", got)
	}
}

func TestRecallQueryOnlyForBackReferences(t *testing.T) {
	for task, want := range map[string]string{
		"Add a retry to the uploader": "",
		"before":                      "",
		"what did we ask before":      "",
		"What did I ask earlier about the retry policy?": "retry policy",
		"Remember the old case with SQLite locking":      "with sqlite locking",
	} {
		if got := contract.RecallQuery(task); got != want {
			t.Errorf("%q: query %q, want %q", task, got, want)
		}
	}
}

func TestPrependRecalledTurnSkipsDuplicatesAndStaysBounded(t *testing.T) {
	var recent []contract.ConversationTurn
	for i := 0; i < contract.MaxRecentTurns; i++ {
		recent = append(recent, contract.ConversationTurn{ID: fmt.Sprintf("turn_%d", i), User: fmt.Sprintf("request %d", i), Assistant: "a"})
	}
	matches := []contract.ConversationTurn{{ID: "dup", User: "request 3", Assistant: "a"}, {ID: "old", User: "ancient request", Assistant: "a"}, {ID: "older", User: "unused", Assistant: "a"}}
	got := contract.PrependRecalledTurn(recent, matches)
	if len(got) != contract.MaxRecentTurns || got[0].ID != "old" || got[1].ID != "turn_1" {
		t.Fatalf("recall: %+v", got)
	}
	if same := contract.PrependRecalledTurn(recent, matches[:1]); len(same) != len(recent) || same[0].ID != "turn_0" {
		t.Fatalf("duplicate was recalled: %+v", same)
	}
}
