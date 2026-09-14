package directexec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"multiharness-core/internal/store"
)

const maxEventBytes = 4 << 20

// stdout has a single writer under OSRunner. Native tool payloads are discarded;
// only the most recent agent response and session ID survive in application state.
type stream struct {
	harness   string
	response  store.DirectResponse
	pending   []byte
	err       error
	completed bool
	blocked   *store.BlockedAction
}

func newStream(harness, session string) *stream {
	return &stream{harness: harness, response: store.DirectResponse{SessionID: session}}
}

func (s *stream) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 && s.err == nil {
		at := bytes.IndexByte(p, '\n')
		end := len(p)
		if at >= 0 {
			end = at
		}
		if len(s.pending)+end > maxEventBytes {
			s.err = errors.New("CLI event exceeded the output limit")
			s.pending = nil
			break
		}
		s.pending = append(s.pending, p[:end]...)
		if at < 0 {
			break
		}
		s.parse(s.pending)
		s.pending = nil
		p = p[at+1:]
	}
	return n, nil
}

func (s *stream) finish() (store.DirectResponse, error) {
	if s.err == nil && len(bytes.TrimSpace(s.pending)) > 0 {
		s.parse(s.pending)
	}
	s.pending = nil
	// OpenCode auto-rejects permission prompts in non-interactive mode, emits a
	// tool_use error followed by step_finish(tool-calls), then exits zero. That
	// is a blocked turn, not successful completion or a truncated transport.
	if s.err == nil && !s.completed && s.blocked != nil {
		s.response.NeedsInput = true
		s.response.Blocked = s.blocked
	}
	if s.err == nil && !s.completed && !s.response.NeedsInput {
		s.err = errors.New("CLI stream ended before turn completion")
	}
	if s.err == nil && s.completed && s.response.SessionID == "" {
		s.err = errors.New("CLI completed without a session ID")
	}
	return s.response, s.err
}

func (s *stream) session(id string) {
	if id == "" {
		return
	}
	r := store.DirectResponse{SessionID: id}
	if r.Validate() != nil {
		s.err = errors.New("CLI returned an invalid session ID")
		return
	}
	if s.response.SessionID != "" && s.response.SessionID != id {
		s.err = errors.New("CLI returned a different session ID")
		return
	}
	s.response.SessionID = id
}

func (s *stream) parse(line []byte) {
	if len(bytes.TrimSpace(line)) == 0 {
		return
	}
	var e struct {
		Type          string            `json:"type"`
		ThreadID      string            `json:"thread_id"`
		SessionID     string            `json:"sessionID"`
		ClaudeSession string            `json:"session_id"`
		Subtype       string            `json:"subtype"`
		IsError       bool              `json:"is_error"`
		Result        string            `json:"result"`
		Denials       []json.RawMessage `json:"permission_denials"`
		Item          struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
		Part struct {
			Type   string `json:"type"`
			Text   string `json:"text"`
			Reason string `json:"reason"`
			Tool   string `json:"tool"`
			State  struct {
				Status string `json:"status"`
				Error  string `json:"error"`
				Input  struct {
					FilePath string `json:"filePath"`
				} `json:"input"`
			} `json:"state"`
		} `json:"part"`
		// Claude system events also use message, but as a string. Decode the
		// assistant object only for assistant events, never for native notices.
		Message json.RawMessage `json:"message"`
	}
	if json.Unmarshal(line, &e) != nil || e.Type == "" {
		s.err = errors.New("CLI returned an invalid event")
		return
	}
	switch s.harness {
	case "codex":
		s.session(e.ThreadID)
		if e.Type == "item.completed" && e.Item.Type == "agent_message" {
			s.response.Text = e.Item.Text
		}
		if e.Type == "turn.completed" {
			s.completed = true
		}
		if e.Type == "turn.failed" || e.Type == "error" {
			s.err = errors.New("Codex reported a failed turn")
		}
	case "opencode":
		s.session(e.SessionID)
		// Only the native error field counts. A web page or other tool output
		// containing the same words must never impersonate a permission denial.
		if e.Type == "tool_use" && e.Part.Type == "tool" && e.Part.State.Status == "error" && e.Part.State.Error == "The user rejected permission to use this specific tool call." {
			tool, target := e.Part.Tool, e.Part.State.Input.FilePath
			if tool != "" && len(tool) <= 128 && len(target) <= 2048 {
				s.blocked = &store.BlockedAction{Tool: tool, Target: target}
			}
		}
		if e.Type == "text" && strings.TrimSpace(e.Part.Text) != "" {
			s.response.Text = e.Part.Text
		}
		if e.Type == "step_start" {
			s.completed = false
			s.blocked = nil // a new model step has continued beyond earlier denials
		}
		if e.Type == "step_finish" && (e.Part.Reason == "stop" || e.Part.Reason == "end_turn") {
			s.completed = true
		}
		if e.Type == "error" {
			s.err = errors.New("OpenCode reported a failed turn")
		}
	case "claude":
		s.session(e.ClaudeSession)
		if e.Type == "assistant" {
			var message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			}
			if json.Unmarshal(e.Message, &message) != nil {
				s.err = errors.New("Claude returned an invalid assistant message")
				return
			}
			var text strings.Builder
			for _, part := range message.Content {
				if part.Type == "text" {
					text.WriteString(part.Text)
				}
			}
			if text.Len() > 0 {
				s.response.Text = text.String()
			}
		}
		if e.Type == "result" {
			if e.Result != "" {
				s.response.Text = e.Result
			}
			s.response.NeedsInput = len(e.Denials) > 0
			s.completed = e.Subtype == "success" && !e.IsError
			if !s.completed && !s.response.NeedsInput {
				s.err = fmt.Errorf("Claude reported an unsuccessful turn")
			}
		}
	}
}
