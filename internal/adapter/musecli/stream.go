package musecli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/agent/structured"
)

const MaxLine = 4 << 20

// Stream retains only the root run's terminal response, not the full event log.
type Stream struct {
	buffer                 []byte
	err                    error
	command, session, text string
	terminal               bool
}

func (s *Stream) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 && s.err == nil {
		part, rest, newline := bytes.Cut(data, []byte{'\n'})
		if len(s.buffer)+len(part) > MaxLine {
			s.err = errors.New("Muse event exceeded 4 MiB")
			break
		}
		s.buffer = append(s.buffer, part...)
		if !newline {
			break
		}
		s.line()
		data = rest
	}
	return n, nil
}

func (s *Stream) line() {
	data := bytes.TrimSpace(s.buffer)
	s.buffer = nil
	if len(data) == 0 {
		return
	}
	if err := structured.ValidateObject(data, "schema_version", "payload_type", "payload_schema_version", "stream", "payload"); err != nil {
		s.err = errors.New("invalid Muse event envelope")
		return
	}
	var e struct {
		Version        int                       `json:"schema_version"`
		PayloadVersion int                       `json:"payload_schema_version"`
		Type           string                    `json:"payload_type"`
		Stream         struct{ Kind, ID string } `json:"stream"`
		Payload        json.RawMessage           `json:"payload"`
	}
	if json.Unmarshal(data, &e) != nil || e.Version != 1 || e.PayloadVersion != 1 {
		s.err = errors.New("unsupported Muse event schema")
		return
	}
	if e.Stream.Kind != "session" {
		return
	}
	if e.Type != "run.lifecycle.started" && !strings.HasPrefix(e.Type, "run.terminal.") {
		return
	}
	if err := structured.ValidateObject(e.Payload, "command_id", "kind", "terminal", "text", "reason"); err != nil {
		s.err = errors.New("invalid Muse run event")
		return
	}
	var p struct {
		Command                string `json:"command_id"`
		Terminal, Text, Reason string
	}
	if json.Unmarshal(e.Payload, &p) != nil {
		s.err = errors.New("invalid Muse terminal response")
		return
	}
	if e.Type == "run.lifecycle.started" {
		if s.command != "" || p.Command == "" || e.Stream.ID == "" {
			s.err = errors.New("ambiguous Muse root run")
			return
		}
		s.command, s.session = p.Command, e.Stream.ID
		return
	}
	if p.Command != s.command || e.Stream.ID != s.session || s.command == "" || s.terminal {
		s.err = errors.New("mismatched or duplicate Muse terminal event")
		return
	}
	s.terminal = true
	if e.Type != "run.terminal.completed" || p.Terminal != "completed" || p.Reason != "" {
		if failure := provider.Text(p.Reason); failure != nil {
			s.err = failure
		} else {
			s.err = errors.New("Muse run did not complete; inspect /failures for the reported reason")
		}
		return
	}
	s.text = p.Text
}

func (s *Stream) Finish() (string, error) {
	if s.err == nil && len(s.buffer) > 0 {
		s.line()
	}
	if s.err != nil {
		return "", s.err
	}
	if !s.terminal || strings.TrimSpace(s.text) == "" {
		return "", errors.New("Muse did not return a complete final response")
	}
	return s.text, nil
}
