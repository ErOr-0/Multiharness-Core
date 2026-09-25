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
	if agent == Muse {
		return museText(data)
	}
	var e struct {
		Type    string          `json:"type"`
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
		Item    struct {
			Type     string          `json:"type"`
			Status   string          `json:"status"`
			Server   string          `json:"server"`
			Tool     string          `json:"tool"`
			Command  string          `json:"command"`
			Text     string          `json:"text"`
			Output   string          `json:"aggregated_output"`
			Error    json.RawMessage `json:"error"`
			ExitCode *int            `json:"exit_code"`
		} `json:"item"`
		Part struct {
			Text  string `json:"text"`
			Tool  string `json:"tool"`
			State struct {
				Output string          `json:"output"`
				Error  json.RawMessage `json:"error"`
			} `json:"state"`
		} `json:"part"`
	}
	if json.Unmarshal(data, &e) != nil {
		return ""
	}
	text := ""
	if e.Type == "error" || e.Type == "turn.failed" {
		text = e.Message
		if message := errorMessage(e.Error); message != "" {
			text = message
		}
		return DisplayText(text)
	}
	if agent == Codex {
		switch e.Item.Type {
		case "agent_message":
			text = e.Item.Text
		case "command_execution":
			if e.Item.ExitCode != nil {
				text = fmt.Sprintf("[shell exit %d; not a validation result]\n", *e.Item.ExitCode)
			}
			text += "$ " + e.Item.Command
			if e.Item.Output != "" {
				text += "\n" + e.Item.Output
			}
		case "mcp_tool_call":
			if e.Item.Status == "failed" {
				text = "MCP tool " + e.Item.Server + "/" + e.Item.Tool
				if message := errorMessage(e.Item.Error); message != "" {
					text += "\n" + message
				}
			}
		case "file_change", "web_search":
			if e.Item.Status == "failed" {
				text = e.Item.Type + " failed"
				if message := errorMessage(e.Item.Error); message != "" {
					text += ": " + message
				}
			}
		}
	} else if agent == OpenCode {
		switch e.Type {
		case "text":
			text = e.Part.Text
		case "tool_use":
			text = e.Part.Tool
			if message := errorMessage(e.Part.State.Error); message != "" {
				text += "\n" + message
			} else if e.Part.State.Output != "" {
				text += "\n" + e.Part.State.Output
			}
		}
	}
	return DisplayText(text)
}

func errorMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var message string
	if json.Unmarshal(raw, &message) == nil {
		return message
	}
	var failure struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &failure) == nil {
		return failure.Message
	}
	return ""
}

// failureSummary avoids command arguments and arbitrary tool output on the
// compact line. The full, filtered provider detail stays behind disclosure.
func failureSummary(agent Agent, data []byte) string {
	if agent == Muse {
		return "Muse tool or run failed"
	}
	var e struct {
		Type string `json:"type"`
		Item struct {
			Type     string `json:"type"`
			ExitCode *int   `json:"exit_code"`
		} `json:"item"`
		Part struct {
			Type string `json:"type"`
			Tool string `json:"tool"`
		} `json:"part"`
	}
	if json.Unmarshal(data, &e) != nil {
		return "tool failed"
	}
	if agent == Codex {
		switch e.Item.Type {
		case "command_execution":
			if e.Item.ExitCode != nil {
				return fmt.Sprintf("command exited %d", *e.Item.ExitCode)
			}
			return "command failed"
		case "mcp_tool_call":
			return "MCP tool failed"
		case "web_search":
			return "web search failed"
		case "file_change":
			return "file change failed"
		}
		return "Codex reported an error"
	}
	if agent == OpenCode && e.Part.Type == "tool" {
		name := DisplayText(e.Part.Tool)
		if name != "" && len(name) <= 40 && !strings.ContainsAny(name, "\n\t") {
			return name + " failed"
		}
	}
	return "tool failed"
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
	text = filterText(text)
	const limit = 8192
	if len(text) > limit {
		text = strings.ToValidUTF8(text[:limit], "") + "\n[output truncated at 8 KiB]"
	}
	return strings.TrimSpace(text)
}

// DetailText preserves the full supported provider event instead of applying
// the transcript's 8 KiB preview limit. Custom publishers are bounded too; if
// oversized, retain both ends so a final diagnostic is not silently discarded.
func DetailText(text string) string {
	text = filterText(text)
	if len(text) > maxLineBytes {
		const marker = "\n[output middle omitted: detail exceeds 1 MiB]\n"
		half := (maxLineBytes - len(marker)) / 2
		text = strings.ToValidUTF8(text[:half], "") + marker + strings.ToValidUTF8(text[len(text)-half:], "")
	}
	return strings.TrimSpace(text)
}

func filterText(text string) string {
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
	return text
}

func failureDetail(event *Event, data []byte) {
	if event.Agent == Muse {
		event.Detailed = true
		event.Error = DetailText(museDetail(data))
		return
	}
	var e struct {
		Type    string          `json:"type"`
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
		Item    struct {
			Type    string          `json:"type"`
			Command string          `json:"command"`
			Output  string          `json:"aggregated_output"`
			Error   json.RawMessage `json:"error"`
		} `json:"item"`
		Part struct {
			State struct {
				Input struct {
					Command string `json:"command"`
				} `json:"input"`
				Output string          `json:"output"`
				Error  json.RawMessage `json:"error"`
			} `json:"state"`
		} `json:"part"`
	}
	if json.Unmarshal(data, &e) != nil {
		return
	}
	event.Detailed = true
	if e.Type == "error" || e.Type == "turn.failed" {
		event.Error = errorMessage(e.Error)
		if event.Error == "" {
			event.Error = e.Message
		}
	} else if event.Agent == Codex {
		event.Command, event.Output, event.Error = e.Item.Command, e.Item.Output, errorMessage(e.Item.Error)
	} else if event.Agent == OpenCode {
		event.Command, event.Output, event.Error = e.Part.State.Input.Command, e.Part.State.Output, errorMessage(e.Part.State.Error)
	}
	event.Command, event.Output, event.Error = DetailText(event.Command), DetailText(event.Output), DetailText(event.Error)
}
