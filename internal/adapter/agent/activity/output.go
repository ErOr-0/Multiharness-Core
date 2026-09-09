package activity

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Only public message/tool fields are displayed. Reasoning, session metadata,
// environment dumps and raw provider envelopes are never selected.
func visibleText(agent Agent, data []byte) string {
	var e struct {
		Type    string          `json:"type"`
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
		Item    struct {
			Type     string `json:"type"`
			Command  string `json:"command"`
			Text     string `json:"text"`
			Output   string `json:"aggregated_output"`
			ExitCode *int   `json:"exit_code"`
		} `json:"item"`
		Part struct {
			Text  string `json:"text"`
			Tool  string `json:"tool"`
			State struct {
				Output string `json:"output"`
			} `json:"state"`
		} `json:"part"`
	}
	if json.Unmarshal(data, &e) != nil {
		return ""
	}
	text := ""
	if e.Type == "error" || e.Type == "turn.failed" {
		text = e.Message
		var message string
		var failure struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(e.Error, &message) == nil {
			text = message
		} else if json.Unmarshal(e.Error, &failure) == nil && failure.Message != "" {
			text = failure.Message
		}
		return DisplayText(text)
	}
	if agent == Codex {
		switch e.Item.Type {
		case "agent_message":
			text = e.Item.Text
		case "command_execution":
			text = "$ " + e.Item.Command
			if e.Item.Output != "" {
				text += "\n" + e.Item.Output
			}
			if e.Item.ExitCode != nil {
				text += fmt.Sprintf("\n[command exit %d]", *e.Item.ExitCode)
			}
		}
	} else if agent == OpenCode {
		switch e.Type {
		case "text":
			text = e.Part.Text
		case "tool_use":
			text = e.Part.Tool + "\n" + e.Part.State.Output
		}
	}
	return DisplayText(text)
}

var displaySecrets = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*(?:bearer|basic)\s+)[^\s"']+`),
	regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|refresh[_-]?token|password|secret)\s*["']?\s*[:=]\s*["']?)[^\s"',}]+`),
	regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{8,}|gh[pousr]_[A-Za-z0-9_]{8,}|github_pat_[A-Za-z0-9_]+)\b`),
}
var terminalEscape = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")

// DisplayText is best-effort credential filtering, not a guarantee that arbitrary
// repository output is non-sensitive. Bound each displayed event and strip
// terminal controls before handing text to the terminal renderer.
func DisplayText(text string) string {
	text = terminalEscape.ReplaceAllString(text, "")
	text = strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && (unicode.IsControl(r) || unicode.In(r, unicode.Cf)) {
			return -1
		}
		return r
	}, text)
	for _, pattern := range displaySecrets {
		replacement := "[redacted]"
		if pattern.NumSubexp() > 0 {
			replacement = "${1}[redacted]"
		}
		text = pattern.ReplaceAllString(text, replacement)
	}
	const limit = 8192
	if len(text) > limit {
		text = strings.ToValidUTF8(text[:limit], "") + "\n[output truncated at 8 KiB]"
	}
	return strings.TrimSpace(text)
}
