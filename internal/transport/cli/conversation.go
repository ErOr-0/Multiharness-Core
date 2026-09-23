package cli

import (
	"reflect"
	"strings"
	"unicode/utf8"

	"multiharness-core/internal/config"
	"multiharness-core/internal/store"
)

const (
	maxRecentTurns = 6
	maxRecentBytes = 24 << 10
	maxTurnText    = 4 << 10
)

// Team agents start fresh processes, so keep a bounded, in-memory transcript
// for later turns and for any model selected by the next workflow stage.
func appendTeamTurn(turns []store.ConversationTurn, task string, output store.TaskOutput) []store.ConversationTurn {
	reply := output.Summary
	switch output.Status {
	case store.TaskStatusAnswered:
		if output.Plan != nil {
			reply = output.Plan.Display()
		}
	case store.TaskStatusApproved, store.TaskStatusRepairLimitReached:
		if output.Implementation != nil {
			reply = output.Implementation.Summary + "\n" + reply
		}
	case store.TaskStatusFailed, store.TaskStatusNeedsInput:
		if output.Failure != nil {
			reply += "\n" + output.Failure.Message
		}
	case store.TaskStatusCancelled:
		return turns
	}
	user, assistant := boundedConversationText(task), boundedConversationText(reply)
	if strings.TrimSpace(user) == "" || strings.TrimSpace(assistant) == "" {
		return turns
	}
	turns = append(turns, store.ConversationTurn{
		User:      user,
		Assistant: assistant,
	})
	for len(turns) > maxRecentTurns || conversationBytes(turns) > maxRecentBytes {
		turns = turns[1:]
	}
	return turns
}

func boundedConversationText(value string) string {
	value = terminalText(value)
	if len(value) <= maxTurnText {
		return value
	}
	const suffix = "… [earlier text truncated]"
	limit := maxTurnText - len(suffix)
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit] + suffix
}

func conversationBytes(turns []store.ConversationTurn) int {
	total := 0
	for _, turn := range turns {
		total += len(turn.User) + len(turn.Assistant)
	}
	return total
}

// Session identity is local to a workspace and the selected native agent.
// Switching away and back starts fresh; sessions never cross provider boundaries.
func sameConversation(a, b config.Config) bool {
	// Increasing a deadline after an interrupted turn must not discard context.
	left, right := a.Implementer, b.Implementer
	left.Timeout, right.Timeout = 0, 0
	// Native permissions are applied on every invocation, including resume.
	left.PermissionPolicy, right.PermissionPolicy = "", ""
	left.Sandbox, right.Sandbox = "", ""
	return a.Mode == b.Mode && a.WorkingDir == b.WorkingDir && reflect.DeepEqual(left, right)
}
