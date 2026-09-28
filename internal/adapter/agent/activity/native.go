package activity

import "encoding/json"

// Native server progress is deliberately an allowlist. Approval details remain
// in the transient approval UI and never enter telemetry or failure history.
func nativeActivity(agent Agent, data []byte) (Event, bool) {
	var m struct {
		Method string `json:"method"`
		Type   string `json:"type"`
		Params struct {
			Item struct {
				Type   string `json:"type"`
				Kind   string `json:"kind"`
				Status string `json:"status"`
				Text   string `json:"text"`
			} `json:"item"`
			Update struct {
				Kind    string `json:"sessionUpdate"`
				Status  string `json:"status"`
				Content struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"update"`
		} `json:"params"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
		} `json:"message"`
	}
	if json.Unmarshal(data, &m) != nil {
		return Event{}, false
	}
	e := Event{Agent: agent}
	switch m.Method {
	case "turn/started":
		e.Kind = TurnStarted
	case "turn/completed":
		e.Kind = StepFinished
	case "item/started", "item/completed":
		i := m.Params.Item
		kind := i.Type
		if kind == "" {
			kind = i.Kind
		}
		switch kind {
		case "agentMessage":
			if m.Method == "item/completed" {
				e.Kind = ResponseReceived
				e.Text = DisplayText(i.Text)
			}
		case "commandExecution", "toolCall", "mcpToolCall":
			e.Kind = ToolRunning
			if m.Method == "item/completed" {
				e.Kind = ToolFinished
			}
			if i.Status == "failed" {
				e.Kind = ToolFailed
			}
		case "fileChange":
			if m.Method == "item/completed" {
				e.Kind = FilesChanged
			}
		}
	case "session/update":
		u := m.Params.Update
		switch u.Kind {
		case "agent_message_chunk":
			e.Kind = ResponseReceived
			if u.Content.Type == "text" {
				e.Text = DisplayText(u.Content.Text)
			}
		case "tool_call":
			e.Kind = ToolRunning
		case "tool_call_update":
			if u.Status == "completed" {
				e.Kind = ToolFinished
			}
			if u.Status == "failed" {
				e.Kind = ToolFailed
			}
		}
	}
	if agent == Claude && m.Type == "assistant" {
		for _, part := range m.Message.Content {
			if part.Type == "text" {
				e.Kind = ResponseReceived
				e.Text = DisplayText(part.Text)
			}
			if part.Type == "tool_use" {
				e.Kind = ToolRunning
			}
		}
	}
	return e, e.Valid()
}
