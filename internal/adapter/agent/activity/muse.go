package activity

import (
	"encoding/json"
	"strings"
)

type museEvent struct {
	Type    string `json:"payload_type"`
	Payload struct {
		Text, Reason string
		Event        struct {
			Kind, Reason string
			Operation    string
		} `json:"event"`
	} `json:"payload"`
}

func museKind(data []byte) Kind {
	var e museEvent
	if json.Unmarshal(data, &e) != nil {
		return ""
	}
	switch e.Type {
	case "run.lifecycle.started":
		return TurnStarted
	case "run.terminal.completed":
		return ResponseReceived
	case "run.terminal.failed", "run.terminal.cancelled", "task.lifecycle.failed":
		return ToolFailed
	case "task.lifecycle.started":
		return ToolRunning
	case "task.lifecycle.completed":
		return ToolFinished
	}
	return ""
}

func museText(data []byte) string { return DisplayText(museDetail(data)) }

func museDetail(data []byte) string {
	var e museEvent
	if json.Unmarshal(data, &e) != nil {
		return ""
	}
	if e.Type == "run.terminal.completed" {
		return e.Payload.Text
	}
	if strings.HasPrefix(e.Type, "run.terminal.") {
		return e.Payload.Reason
	}
	if e.Type == "task.lifecycle.failed" {
		return e.Payload.Event.Reason
	}
	return ""
}
