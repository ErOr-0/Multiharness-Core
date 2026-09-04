package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

const (
	maximumEventBytes  = 1024 * 1024
	maximumErrorLength = 4096
)

type wireEvent struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionID"`
	Part      json.RawMessage `json:"part"`
	Error     json.RawMessage `json:"error"`
}

type wirePart struct {
	Type  string        `json:"type"`
	Text  string        `json:"text"`
	Tool  string        `json:"tool"`
	State wireToolState `json:"state"`
}

type wireToolState struct {
	Status string `json:"status"`
}

type parsedEvents struct {
	sessionID  string
	finalText  string
	agentError string
}

type eventStream struct {
	mu                sync.Mutex
	expectedSessionID string
	sink              ProgressSink
	buffer            []byte
	parsed            parsedEvents
	eventsSeen        int
	parseErr          error
}

func newEventStream(expectedSessionID string, sink ProgressSink) *eventStream {
	if sink == nil {
		sink = discardProgressSink{}
	}
	return &eventStream{expectedSessionID: expectedSessionID, sink: sink}
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
	if strings.TrimSpace(stream.parsed.finalText) == "" && stream.parsed.agentError == "" {
		return parsedEvents{}, fmt.Errorf("event stream did not contain a final text response")
	}
	return stream.parsed, nil
}

func (stream *eventStream) parseLine(line []byte) error {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil
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
		stream.sink.Publish(ProgressEvent{Type: ProgressStepStarted, SessionID: event.SessionID})
	case "step_finish":
		if _, err := decodePartAs(event, "step-finish"); err != nil {
			return err
		}
		stream.sink.Publish(ProgressEvent{Type: ProgressStepFinished, SessionID: event.SessionID})
	case "tool_use":
		part, err := decodePartAs(event, "tool")
		if err != nil {
			return err
		}
		if strings.TrimSpace(part.Tool) == "" {
			return fmt.Errorf("JSON event %q tool is missing or blank", event.Type)
		}
		if part.State.Status != "completed" && part.State.Status != "error" {
			return fmt.Errorf("JSON event %q has unsupported tool status %q", event.Type, part.State.Status)
		}
		stream.sink.Publish(ProgressEvent{
			Type:      ProgressToolFinished,
			SessionID: event.SessionID,
			Tool:      part.Tool,
			Status:    part.State.Status,
		})
	case "text":
		part, err := decodePartAs(event, "text")
		if err != nil {
			return err
		}
		if strings.TrimSpace(part.Text) != "" {
			stream.parsed.finalText = part.Text
		}
	case "error":
		stream.parsed.agentError = describeEventError(event.Error)
	}
	return nil
}

func decodePartAs(event wireEvent, expectedType string) (wirePart, error) {
	if len(event.Part) == 0 || bytes.Equal(bytes.TrimSpace(event.Part), []byte("null")) {
		return wirePart{}, fmt.Errorf("JSON event %q part is missing or null", event.Type)
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

func describeEventError(data json.RawMessage) string {
	if len(data) == 0 || bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return "unspecified error"
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return boundedString(string(data), maximumErrorLength)
	}
	if message := findErrorMessage(value); message != "" {
		return boundedString(message, maximumErrorLength)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "unspecified error"
	}
	return boundedString(string(encoded), maximumErrorLength)
}

func findErrorMessage(value any) string {
	object, ok := value.(map[string]any)
	if !ok {
		if text, ok := value.(string); ok {
			return text
		}
		return ""
	}
	if message, ok := object["message"].(string); ok && strings.TrimSpace(message) != "" {
		return message
	}
	if nested, exists := object["data"]; exists {
		if message := findErrorMessage(nested); message != "" {
			return message
		}
	}
	if name, ok := object["name"].(string); ok {
		return name
	}
	return ""
}

func boundedString(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
