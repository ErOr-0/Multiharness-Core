package main

import "encoding/json"

var commandCodeAdapter = &adapter{
	Name: "commandcode", Title: "Command Code", Bin: "command-code",
	Resume:       true,
	Instructions: "AGENTS.md",
	Login:        "command-code login",
	LoginCheck:   []string{"status", "--json"},
	Docs:         "https://commandcode.ai/docs/quickstart",
	Install: []installer{
		{Needs: "npm", Command: []string{"npm", "install", "--global", "command-code@latest"}},
	},
	Notes: []string{
		"Command Code accepts file edits but has no per-command allow list; an action it may not take ends the job with `needs_permission`.",
	},
	args:   commandCodeArgs,
	parse:  parseCommandCode,
	finish: finishCommandCode,
}

// The prompt arrives on stdin. command-code is the executable name that works
// on every system (on Windows, cmd is the shell).
func commandCodeArgs(req Request, _ string) []string {
	mode := "accept-edits"
	if req.Sandbox == "read-only" {
		mode = "plan"
	}
	args := []string{"--print", "--output-format", "json", "--permission-mode", mode}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Reasoning != "" {
		args = append(args, "--effort", req.Reasoning)
	}
	if req.Session != "" {
		args = append(args, "--resume", req.Session)
	}
	return append(args, req.ExtraArgs...)
}

// parseCommandCode reads NDJSON: progress events, then one result line.
// Unknown event types are ignored, as the format's documentation asks.
func parseCommandCode(t *tracker, line []byte) {
	var e struct {
		Type      string          `json:"type"`
		Subtype   string          `json:"subtype"`
		SessionID string          `json:"sessionId"`
		Usage     json.RawMessage `json:"usage"`
		FinalText string          `json:"finalText"`
		Error     json.RawMessage `json:"error"`
		Event     struct {
			ToolName    string `json:"toolName"`
			Description string `json:"description"`
		} `json:"event"`
	}
	if json.Unmarshal(line, &e) != nil {
		return
	}
	if e.SessionID != "" {
		t.session = e.SessionID
	}
	switch e.Type {
	case "event":
		if e.Event.ToolName != "" {
			t.note(e.Event.ToolName + " " + e.Event.Description)
		}
	case "result":
		t.say(e.FinalText)
		t.usage = e.Usage
		switch e.Subtype {
		case "success":
			t.done = true
		case "max_turns":
			t.turnFailed = "Command Code reached its turn limit; raise it with `-- --max-turns N`"
		default:
			t.lastError = textOf(e.Error)
			t.turnFailed = firstNonEmpty(t.lastError, "Command Code ended with an error")
		}
	}
}

var commandCodeExits = map[int]string{
	3:  "Command Code is not signed in; run `command-code login` or set COMMAND_CODE_API_KEY",
	4:  "Command Code stopped because an action was not permitted",
	5:  "Command Code was rate limited",
	6:  "Command Code could not reach its server",
	7:  "Command Code's server returned an error",
	8:  "Command Code reached its turn limit; raise it with `-- --max-turns N`",
	9:  "the model sent no response",
	10: "the Command Code account is out of credits",
}

func finishCommandCode(t *tracker, code int) {
	hint, ok := commandCodeExits[code]
	if !ok {
		return
	}
	if t.lastError != "" && t.lastError != hint {
		hint += ": " + t.lastError
	}
	t.turnFailed, t.done = hint, false
	t.needsPermission = code == 4
}
