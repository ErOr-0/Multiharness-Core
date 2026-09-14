package sessionexec

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/store"
)

const maximumEventBytes = 1024 * 1024

type wireEvent struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionID"`
	Part      json.RawMessage `json:"part"`
}

type wirePart struct {
	Reason string          `json:"reason"`
	Type   string          `json:"type"`
	Text   string          `json:"text"`
	Tool   string          `json:"tool"`
	State  json.RawMessage `json:"state"`
}

type wireToolState struct {
	Status string          `json:"status"`
	Error  string          `json:"error"`
	Input  json.RawMessage `json:"input"`
}

type parsedEvents struct {
	completed   bool
	blocked     *store.PermissionDenied
	sessionID   string
	finalText   string
	agentFailed bool
}

type eventStream struct {
	mu                sync.Mutex
	expectedSessionID string
	buffer            []byte
	parsed            parsedEvents
	eventsSeen        int
	parseErr          error
}

func newEventStream(expectedSessionID string) *eventStream {
	return &eventStream{expectedSessionID: expectedSessionID}
}

func (stream *eventStream) Write(data []byte) (int, error) {
	stream.mu.Lock()
	defer stream.mu.Unlock()

	if stream.parseErr != nil || len(data) == 0 {
		return len(data), nil
	}
	stream.buffer = append(stream.buffer, data...)
	for {
		newline := bytes.IndexByte(stream.buffer, '\n')
		if newline < 0 {
			if len(stream.buffer) > maximumEventBytes {
				stream.parseErr = fmt.Errorf("JSON event exceeds %d bytes", maximumEventBytes)
				stream.buffer = nil
			}
			return len(data), nil
		}
		line := stream.buffer[:newline]
		stream.buffer = stream.buffer[newline+1:]
		if len(line) > maximumEventBytes {
			stream.parseErr = fmt.Errorf("JSON event exceeds %d bytes", maximumEventBytes)
			stream.buffer = nil
			return len(data), nil
		}
		if err := stream.parseLine(line); err != nil {
			stream.parseErr = err
			stream.buffer = nil
			return len(data), nil
		}
	}
}

func (stream *eventStream) session() string {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.parsed.sessionID == "" {
		return stream.expectedSessionID
	}
	return stream.parsed.sessionID
}

func (stream *eventStream) finish() (parsedEvents, error) {
	stream.mu.Lock()
	defer stream.mu.Unlock()

	if stream.parseErr == nil && len(bytes.TrimSpace(stream.buffer)) > 0 {
		if len(stream.buffer) > maximumEventBytes {
			stream.parseErr = fmt.Errorf("JSON event exceeds %d bytes", maximumEventBytes)
		} else {
			stream.parseErr = stream.parseLine(stream.buffer)
		}
	}
	stream.buffer = nil
	if stream.parseErr != nil {
		return parsedEvents{}, stream.parseErr
	}
	if stream.eventsSeen == 0 {
		return parsedEvents{}, fmt.Errorf("event stream is empty")
	}
	if stream.parsed.sessionID == "" {
		return parsedEvents{}, fmt.Errorf("event stream did not report a session ID")
	}
	if !stream.parsed.agentFailed && !stream.parsed.completed && stream.parsed.blocked != nil {
		return parsedEvents{}, stream.parsed.blocked
	}
	if strings.TrimSpace(stream.parsed.finalText) == "" && !stream.parsed.agentFailed {
		return parsedEvents{}, fmt.Errorf("event stream did not contain a final text response")
	}
	if !stream.parsed.agentFailed && !stream.parsed.completed {
		return parsedEvents{}, fmt.Errorf("OpenCode stream ended before turn completion")
	}
	return stream.parsed, nil
}

func (stream *eventStream) parseLine(line []byte) error {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil
	}

	if err := structured.ValidateObject(line, "type", "sessionID", "part", "error"); err != nil {
		return fmt.Errorf("decode JSON event: %w", err)
	}
	var event wireEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return fmt.Errorf("decode JSON event: %w", err)
	}
	if strings.TrimSpace(event.Type) == "" {
		return fmt.Errorf("JSON event type is missing or blank")
	}
	if strings.TrimSpace(event.SessionID) == "" {
		return fmt.Errorf("JSON event %q sessionID is missing or blank", event.Type)
	}
	if err := validateSessionID(event.SessionID); err != nil {
		return fmt.Errorf("JSON event %q sessionID is invalid: %w", event.Type, err)
	}
	if stream.expectedSessionID != "" && event.SessionID != stream.expectedSessionID {
		return fmt.Errorf(
			"JSON event sessionID %q does not match resumed session %q",
			event.SessionID,
			stream.expectedSessionID,
		)
	}
	if stream.parsed.sessionID != "" && event.SessionID != stream.parsed.sessionID {
		return fmt.Errorf(
			"JSON event sessionID changed from %q to %q",
			stream.parsed.sessionID,
			event.SessionID,
		)
	}
	stream.parsed.sessionID = event.SessionID
	stream.eventsSeen++

	switch event.Type {
	case "step_start":
		if _, err := decodePartAs(event, "step-start"); err != nil {
			return err
		}
		stream.parsed.completed = false
		stream.parsed.blocked = nil
		stream.parsed.finalText = "" // Earlier progress is not this step's final response.
	case "step_finish":
		part, err := decodePartAs(event, "step-finish")
		if err != nil {
			return err
		}
		stream.parsed.completed = part.Reason == "stop" || part.Reason == "end_turn"
	case "tool_use":
		part, err := decodePartAs(event, "tool")
		if err != nil {
			return err
		}
		if strings.TrimSpace(part.Tool) == "" {
			return fmt.Errorf("JSON event %q tool is missing or blank", event.Type)
		}
		if err := structured.ValidateObject(part.State, "status", "error", "input"); err != nil {
			return fmt.Errorf("decode JSON tool state: %w", err)
		}
		var state wireToolState
		if err := json.Unmarshal(part.State, &state); err != nil {
			return fmt.Errorf("decode JSON tool state: %w", err)
		}
		if state.Status != "completed" && state.Status != "error" {
			return fmt.Errorf("JSON event %q has unsupported tool status %q", event.Type, state.Status)
		}
		if state.Status == "error" && state.Error == "The user rejected permission to use this specific tool call." {
			var input struct {
				FilePath string `json:"filePath"`
			}
			if len(state.Input) > 0 {
				if err := structured.ValidateObject(state.Input, "filePath"); err != nil {
					return err
				}
				if err := json.Unmarshal(state.Input, &input); err != nil {
					return fmt.Errorf("invalid tool input")
				}
			}
			denied := &store.PermissionDenied{SessionID: event.SessionID, Action: store.BlockedAction{Tool: part.Tool, Target: input.FilePath}}
			if err := denied.Validate(); err != nil {
				return fmt.Errorf("invalid permission denial evidence")
			}
			stream.parsed.blocked = denied
		}
	case "text":
		part, err := decodePartAs(event, "text")
		if err != nil {
			return err
		}
		if strings.TrimSpace(part.Text) != "" {
			stream.parsed.finalText = part.Text
		}
	case "error":
		stream.parsed.agentFailed = true
	}
	return nil
}

func decodePartAs(event wireEvent, expectedType string) (wirePart, error) {
	if len(event.Part) == 0 || bytes.Equal(bytes.TrimSpace(event.Part), []byte("null")) {
		return wirePart{}, fmt.Errorf("JSON event %q part is missing or null", event.Type)
	}
	if err := structured.ValidateObject(event.Part, "type", "text", "tool", "state", "reason"); err != nil {
		return wirePart{}, fmt.Errorf("decode JSON event part: %w", err)
	}
	var part wirePart
	if err := json.Unmarshal(event.Part, &part); err != nil {
		return wirePart{}, fmt.Errorf("decode JSON event %q part: %w", event.Type, err)
	}
	if part.Type != expectedType {
		return wirePart{}, fmt.Errorf(
			"JSON event %q part type %q does not match %q",
			event.Type,
			part.Type,
			expectedType,
		)
	}
	return part, nil
}

func validateSessionID(sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if strings.TrimSpace(sessionID) != sessionID || strings.ContainsAny(sessionID, "\t\r\n\x00") {
		return fmt.Errorf("must not contain surrounding whitespace, control whitespace, or NUL")
	}
	if strings.HasPrefix(sessionID, "-") {
		return fmt.Errorf("must not begin with a flag prefix")
	}
	return nil
}

// unwrapJSONFence accepts the presentation wrapper some OpenCode models add to
// their final response. Only a complete, standalone ```json or unlabelled fence
// is removed, once. The caller must still strictly decode and validate the JSON;
// this does not extract JSON from prose, repair it, or reinterpret its contents.
func unwrapJSONFence(data []byte) []byte {
	trimmed := bytes.Trim(data, " \t\r\n")
	header, rest, ok := bytes.Cut(trimmed, []byte("\n"))
	if !ok {
		return data
	}
	header = bytes.TrimRight(header, " \t\r")
	if !bytes.Equal(header, []byte("```json")) && !bytes.Equal(header, []byte("```")) {
		return data
	}
	lastLine := bytes.LastIndexByte(rest, '\n')
	if lastLine < 0 || !bytes.Equal(rest[lastLine+1:], []byte("```")) {
		return data
	}
	return rest[:lastLine]
}
