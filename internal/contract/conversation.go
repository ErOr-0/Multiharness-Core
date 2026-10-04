package contract

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Conversation window limits. TaskInput.Validate enforces the same bounds, so a
// window built here is always an acceptable handoff.
const (
	MaxRecentTurns     = 6
	MaxRecentTurnBytes = 24 << 10
	MaxTurnTextBytes   = 4 << 10
)

const turnTruncatedSuffix = "… [remaining text truncated]"

// PlainText removes control and format characters (keeping newlines and tabs)
// from untrusted text before it is shown to a person or handed to an agent.
func PlainText(value string) string {
	return strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.In(r, unicode.Cf) {
			return -1
		}
		return r
	}, value)
}

// BoundTurnText sanitises one side of an exchange and keeps its leading
// MaxTurnTextBytes, never splitting a character.
func BoundTurnText(value string) string {
	value = PlainText(value)
	if len(value) <= MaxTurnTextBytes {
		return value
	}
	limit := MaxTurnTextBytes - len(turnTruncatedSuffix)
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit] + turnTruncatedSuffix
}

// ConversationBytes is the size TaskInput.Validate counts against the window.
func ConversationBytes(turns []ConversationTurn) int {
	total := 0
	for _, turn := range turns {
		total += len(turn.User) + len(turn.Assistant)
	}
	return total
}

// AppendTurn records a completed exchange and drops the oldest turns that no
// longer fit. Team agents start fresh processes, so this bounded window is the
// only conversation any later stage or model sees. Cancelled runs add nothing.
func AppendTurn(turns []ConversationTurn, task string, output TaskOutput) []ConversationTurn {
	reply := output.Summary
	switch output.Status {
	case TaskStatusAnswered:
		if output.Plan != nil {
			reply = output.Plan.Display()
		}
	case TaskStatusApproved, TaskStatusRepairLimitReached:
		if output.Implementation != nil {
			reply = output.Implementation.Summary + "\n" + reply
		}
	case TaskStatusFailed, TaskStatusNeedsInput:
		if output.Failure != nil {
			reply += "\n" + output.Failure.Message
		}
	case TaskStatusCancelled:
		return turns
	}
	user, assistant := BoundTurnText(task), BoundTurnText(reply)
	if strings.TrimSpace(user) == "" || strings.TrimSpace(assistant) == "" {
		return turns
	}
	turns = append(turns, ConversationTurn{User: user, Assistant: assistant})
	for len(turns) > MaxRecentTurns || ConversationBytes(turns) > MaxRecentTurnBytes {
		turns = turns[1:]
	}
	return turns
}

// SelectRecentTurns gives every provider the same bounded window. Keyword
// guesses cannot determine whether an earlier user constraint still applies,
// and some providers have no shell tool with which to fetch omitted records;
// only a bare greeting is sent without history.
func SelectRecentTurns(task string, turns []ConversationTurn) []ConversationTurn {
	switch strings.ToLower(strings.TrimSpace(task)) {
	case "hi", "hello", "hey", "hi!", "hello!":
		return nil
	}
	return append([]ConversationTurn(nil), turns[max(0, len(turns)-MaxRecentTurns):]...)
}

var recallStopWords = map[string]bool{"what": true, "did": true, "i": true, "we": true, "ask": true, "asked": true, "before": true, "earlier": true, "previous": true, "remember": true, "about": true, "the": true, "this": true, "that": true, "case": true, "old": true, "long": true, "ago": true, "please": true, "you": true, "can": true, "me": true}

// RecallQuery returns archive search terms when a task refers back to an older
// exchange, or "" when it does not. The trigger is a deliberately conservative
// English heuristic: a miss only means the agent fetches the record itself
// with magent context get.
func RecallQuery(task string) string {
	lower := strings.ToLower(task)
	refersBack := false
	for _, cue := range []string{"earlier", "previous", "remember", "before", "old case"} {
		refersBack = refersBack || strings.Contains(lower, cue)
	}
	if !refersBack || len(strings.Fields(task)) < 3 {
		return ""
	}
	var words []string
	for _, word := range strings.FieldsFunc(lower, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
		if len(word) >= 3 && !recallStopWords[word] {
			words = append(words, word)
		}
	}
	return strings.Join(words, " ")
}

// PrependRecalledTurn places the best archived match that is not already in
// the window ahead of it, evicting the oldest turns to stay within bounds.
func PrependRecalledTurn(recent, matches []ConversationTurn) []ConversationTurn {
	for _, match := range matches {
		older := ConversationTurn{ID: match.ID, User: BoundTurnText(match.User), Assistant: BoundTurnText(match.Assistant)}
		duplicate := false
		for _, existing := range recent {
			duplicate = duplicate || existing.User == older.User
		}
		if duplicate {
			continue
		}
		if len(recent) == MaxRecentTurns {
			recent = recent[1:]
		}
		recent = append([]ConversationTurn{older}, recent...)
		for ConversationBytes(recent) > MaxRecentTurnBytes {
			recent = recent[1:]
		}
		break
	}
	return recent
}
