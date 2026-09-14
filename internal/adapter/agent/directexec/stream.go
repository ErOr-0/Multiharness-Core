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
		} `json:"part"`
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
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
		if e.Type == "text" && strings.TrimSpace(e.Part.Text) != "" {
			s.response.Text = e.Part.Text
		}
		if e.Type == "step_start" {
			s.completed = false
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
			var text strings.Builder
			for _, part := range e.Message.Content {
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
