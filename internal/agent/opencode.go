package agent

import (
	"encoding/json"
	"strings"
)

var OpenCode = &Adapter{
	Name: "opencode", Title: "OpenCode", Bin: "opencode",
	Resume:       true,
	Instructions: "AGENTS.md",
	Login:        "opencode auth login",
	Docs:         "https://opencode.ai/v2/docs/",
	ModelList:    []string{"models"},
	Install: []Installer{
		{Needs: "npm", Command: []string{"npm", "install", "--global", "@opencode/cli"}},
		{GOOS: "!windows", Needs: "bash", Command: []string{"bash", "-c", "curl -fsSL https://opencode.ai/v2/install | bash"}},
	},
	Notes: []string{
		"OpenCode rejects every permission prompt when it runs headless, so it can only do what opencode.json allows. To let it act without asking, pass `--auto` as an extra argument (extra_args in delegate, after -- on the command line); a rejected action is listed in `permission_denied`.",
	},
	args:   opencodeArgs,
	parse:  parseOpenCode,
	finish: finishOpenCode,
}

// The prompt arrives on stdin and the workspace is the process directory, which
// works with OpenCode v1 and v2. A reasoning variant is part of the v2 model
// name (provider/model#variant).
func opencodeArgs(req Request, _ string) []string {
	args := []string{"run", "--format", "json"}
	if req.Sandbox == "read-only" {
		args = append(args, "--agent", "plan")
	}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Session != "" {
		args = append(args, "--session", req.Session)
	}
	return append(args, req.ExtraArgs...)
}

// parseOpenCode reads `opencode run --format json` events. The answer is the
// text after the last tool call. A denied tool call with no later step means
// OpenCode gave up on it.
func parseOpenCode(t *Tracker, line []byte) {
	var e struct {
		Type      string          `json:"type"`
		SessionID string          `json:"sessionID"`
		Error     json.RawMessage `json:"error"`
		Part      struct {
			Text  string  `json:"text"`
			Tool  string  `json:"tool"`
			Cost  float64 `json:"cost"`
			State struct {
				Status   string          `json:"status"`
				Error    string          `json:"error"`
				Input    json.RawMessage `json:"input"`
				Metadata struct {
					Exit *int `json:"exit"`
				} `json:"metadata"`
			} `json:"state"`
		} `json:"part"`
	}
	if json.Unmarshal(line, &e) != nil {
		return
	}
	if e.SessionID != "" {
		t.session = e.SessionID
	}
	part := e.Part
	switch e.Type {
	case "text":
		if strings.TrimSpace(part.Text) == "" {
			break
		}
		if t.afterTool || t.message == "" {
			t.message = part.Text
		} else {
			t.message += "\n\n" + part.Text
		}
		t.afterTool = false
		t.note("said: " + part.Text)
	case "tool_use":
		t.afterTool = true
		switch {
		case part.State.Status == "error" && strings.Contains(part.State.Error, "rejected permission"):
			denied := describeTool(part.Tool, part.State.Input)
			t.denied = append(t.denied, denied)
			t.note("denied " + denied)
			t.blocked = true
		case part.State.Status == "error":
			t.note(describeTool(part.Tool, part.State.Input) + " failed")
		case strings.EqualFold(part.Tool, "bash"):
			var in toolInput
			_ = json.Unmarshal(part.State.Input, &in)
			t.command(in.Command, part.State.Metadata.Exit)
		case part.State.Status == "completed":
			// Only a finished tool call counts as a change; a pending or
			// cancelled one may never have happened.
			t.tool(part.Tool, part.State.Input)
		default:
			t.note(describeTool(part.Tool, part.State.Input))
		}
	case "step_start":
		t.blocked = false
	case "step_finish":
		t.cost += part.Cost
	case "error":
		t.turnFailed = FirstNonEmpty(textOf(e.Error), "OpenCode reported an error")
	}
}

// OpenCode has no completion event: exit status 0 is success.
func finishOpenCode(t *Tracker, code int) {
	t.done = code == 0 && !t.blocked
	if t.done {
		t.markDone()
	}
}
