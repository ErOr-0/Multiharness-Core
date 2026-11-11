package agent

import (
	"encoding/json"
	"strings"
)

var Claude = &Adapter{
	Name: "claude", Title: "Claude Code", Bin: "claude",
	Resume:       true,
	Instructions: "CLAUDE.md",
	Login:        "claude auth login",
	LoginCheck:   []string{"auth", "status"},
	Docs:         "https://code.claude.com/docs/en/setup",
	Install: []Installer{
		{GOOS: "windows", Needs: "powershell", Command: []string{"powershell", "-NoProfile", "-Command", "irm https://claude.ai/install.ps1 | iex"}},
		{GOOS: "!windows", Needs: "bash", Command: []string{"bash", "-c", "curl -fsSL https://claude.ai/install.sh | bash"}},
	},
	Notes: []string{
		"Claude Code runs only the shell commands you allow, such as `Bash(go test:*)` (allow in delegate, --allow on the command line); anything else is denied and listed in `permission_denied`.",
	},
	args:  claudeArgs,
	parse: parseClaude,
}

// Print mode cannot ask, so anything outside the allowed tools is denied and
// reported in permission_denials. The prompt arrives on stdin.
func claudeArgs(req Request, _ string) []string {
	args := []string{"--print", "--output-format", "stream-json", "--verbose"}
	tools := []string{"Read", "Glob", "Grep"}
	if req.Sandbox == "read-only" {
		args = append(args, "--permission-mode", "default", "--disallowedTools", "Edit,Write,NotebookEdit")
	} else {
		args = append(args, "--permission-mode", "acceptEdits")
		tools = append(tools, "Edit", "Write")
	}
	args = append(args, "--allowedTools", strings.Join(append(tools, req.Allow...), ","))
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

// parseClaude reads Claude Code's stream-json events.
func parseClaude(t *Tracker, line []byte) {
	var e struct {
		Type      string          `json:"type"`
		Subtype   string          `json:"subtype"`
		SessionID string          `json:"session_id"`
		IsError   bool            `json:"is_error"`
		Result    string          `json:"result"`
		Usage     json.RawMessage `json:"usage"`
		Cost      float64         `json:"total_cost_usd"`
		Message   json.RawMessage `json:"message"`
		Denials   []struct {
			ToolName  string          `json:"tool_name"`
			ToolInput json.RawMessage `json:"tool_input"`
		} `json:"permission_denials"`
	}
	if json.Unmarshal(line, &e) != nil {
		return
	}
	if e.SessionID != "" {
		t.session = e.SessionID
	}
	switch e.Type {
	case "assistant":
		var m struct {
			Content []struct {
				Type  string          `json:"type"`
				ID    string          `json:"id"`
				Text  string          `json:"text"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		}
		if json.Unmarshal(e.Message, &m) != nil {
			return
		}
		for _, c := range m.Content {
			switch c.Type {
			case "text":
				t.say(c.Text)
			case "tool_use":
				// A file an edit tool targets is only listed once its result
				// shows the edit actually happened.
				if path := editTarget(c.Name, c.Input); path != "" {
					t.deferEdit(c.ID, path)
					t.note(strings.ToLower(c.Name) + " " + t.rel(path))
				} else {
					t.tool(c.Name, c.Input)
				}
			}
		}
	case "user":
		var m struct {
			Content []struct {
				Type      string `json:"type"`
				ToolUseID string `json:"tool_use_id"`
				IsError   bool   `json:"is_error"`
			} `json:"content"`
		}
		if json.Unmarshal(e.Message, &m) != nil {
			return
		}
		for _, c := range m.Content {
			if c.Type == "tool_result" {
				t.settleEdit(c.ToolUseID, c.IsError)
			}
		}
	case "result":
		if e.Result != "" {
			t.message = e.Result
		}
		t.usage, t.cost = e.Usage, e.Cost
		for _, d := range e.Denials {
			t.denied = append(t.denied, describeTool(d.ToolName, d.ToolInput))
		}
		if e.Subtype == "success" && !e.IsError {
			t.markDone()
		} else {
			t.turnFailed = FirstNonEmpty(e.Result, t.adapter.Title+" ended with "+FirstNonEmpty(e.Subtype, "an error"))
		}
	}
}
