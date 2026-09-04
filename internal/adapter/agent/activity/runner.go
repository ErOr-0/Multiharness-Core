// Package activity translates optional CLI telemetry into allowlisted labels.
// It never decides task success, interprets response contents, or retries work.
package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"

	"multiharness-core/internal/adapter/process"
)

type Agent string

const (
	Codex    Agent = "codex"
	OpenCode Agent = "opencode"
)

type Kind string

const (
	Starting         Kind = "starting"
	TurnStarted      Kind = "turn_started"
	CommandRunning   Kind = "command_running"
	CommandFinished  Kind = "command_finished"
	FilesChanged     Kind = "files_changed"
	ToolRunning      Kind = "tool_running"
	ToolFinished     Kind = "tool_finished"
	ToolFailed       Kind = "tool_failed"
	ResponseReceived Kind = "response_received"
	StepFinished     Kind = "step_finished"
)

// Event deliberately has no text, command, path, session ID or reasoning field.
type Event struct {
	Agent Agent `json:"agent"`
	Kind  Kind  `json:"activity"`
}

func (e Event) Valid() bool {
	if e.Agent != Codex && e.Agent != OpenCode {
		return false
	}
	switch e.Kind {
	case Starting, TurnStarted, CommandRunning, CommandFinished, FilesChanged, ToolRunning, ToolFinished, ToolFailed, ResponseReceived, StepFinished:
		return true
	}
	return false
}

type ProcessRunner interface {
	Run(context.Context, process.Command) (process.Result, error)
}

type Runner struct {
	Runner ProcessRunner
	Agent  Agent
	// Observe must return promptly. Presentation coalesces updates without I/O
	// on the subprocess reader; telemetry is not a lossless audit stream.
	Observe func(Event)
}

func (r Runner) Run(ctx context.Context, command process.Command) (process.Result, error) {
	// Runtime discovery/help probes are local metadata, not agent activity.
	if r.Observe == nil || len(command.Args) == 0 ||
		!((r.Agent == Codex && command.Args[0] == "exec" && slices.Contains(command.Args, "--json")) ||
			(r.Agent == OpenCode && command.Args[0] == "run" && slices.Contains(command.Args, "--format"))) {
		return r.Runner.Run(ctx, command)
	}
	if ctx != nil && ctx.Err() != nil {
		return r.Runner.Run(ctx, command)
	}
	stream := &observer{agent: r.Agent, publish: r.Observe}
	r.Observe(Event{Agent: r.Agent, Kind: Starting})
	if command.Stdout == nil {
		command.Stdout = stream
	} else {
		command.Stdout = io.MultiWriter(command.Stdout, stream)
	}
	result, err := r.Runner.Run(ctx, command)
	stream.finish()
	return result, err
}

const maxLineBytes = 1 << 20

type observer struct {
	agent      Agent
	publish    func(Event)
	buffer     []byte
	discarding bool
}

func (o *observer) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 {
		part, rest, newline := bytes.Cut(data, []byte{'\n'})
		if !o.discarding {
			if len(o.buffer)+len(part) > maxLineBytes {
				o.buffer, o.discarding = nil, true
			} else {
				o.buffer = append(o.buffer, part...)
			}
		}
		if !newline {
			break
		}
		o.finish()
		o.discarding = false
		data = rest
	}
	return n, nil
}

func (o *observer) finish() {
	if !o.discarding {
		if kind := decode(o.agent, o.buffer); kind != "" {
			o.publish(Event{Agent: o.agent, Kind: kind})
		}
	}
	o.buffer = nil
}

// Only known event metadata is interpreted; unknown/malformed/oversized events
// are ignored for display. Existing provider and final-response parsers remain
// authoritative. In particular, turn/step completion never means approval.
func decode(agent Agent, data []byte) Kind {
	var event struct {
		Type string `json:"type"`
		Item struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"item"`
		Part struct {
			Type  string `json:"type"`
			State struct {
				Status string `json:"status"`
			} `json:"state"`
		} `json:"part"`
	}
	if json.Unmarshal(data, &event) != nil {
		return ""
	}
	if agent == Codex {
		switch event.Type {
		case "turn.started":
			return TurnStarted
		case "turn.completed":
			return StepFinished
		case "item.started", "item.updated", "item.completed":
			done := event.Type == "item.completed"
			switch event.Item.Type {
			case "command_execution":
				if event.Item.Status == "failed" {
					return ToolFailed
				}
				if done {
					return CommandFinished
				}
				return CommandRunning
			case "file_change":
				if event.Item.Status == "failed" {
					return ToolFailed
				}
				if done {
					return FilesChanged
				}
			case "mcp_tool_call", "web_search":
				if event.Item.Status == "failed" {
					return ToolFailed
				}
				if done {
					return ToolFinished
				}
				return ToolRunning
			case "agent_message":
				if done {
					return ResponseReceived
				}
			}
		}
	} else if agent == OpenCode {
		switch event.Type {
		case "step_start":
			if event.Part.Type == "step-start" {
				return TurnStarted
			}
		case "step_finish":
			if event.Part.Type == "step-finish" {
				return StepFinished
			}
		case "text":
			if event.Part.Type == "text" {
				return ResponseReceived
			}
		case "tool_use":
			if event.Part.Type != "tool" {
				return ""
			}
			switch event.Part.State.Status {
			case "error":
				return ToolFailed
			case "completed":
				return ToolFinished
			}
		}
	}
	return ""
}
