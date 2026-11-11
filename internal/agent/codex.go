package agent

import (
	"encoding/json"
	"strconv"
)

var Codex = &Adapter{
	Name: "codex", Title: "Codex CLI", Bin: "codex",
	Resume:       true,
	Instructions: "AGENTS.md",
	Login:        "codex login",
	LoginCheck:   []string{"login", "status"},
	Docs:         "https://developers.openai.com/codex/cli",
	Install: []Installer{
		{Needs: "npm", Command: []string{"npm", "install", "--global", "@openai/codex"}},
		{GOOS: "darwin", Needs: "brew", Command: []string{"brew", "install", "--cask", "codex"}},
	},
	Notes: []string{
		"Codex runs commands without network access. If a check needs the network, pass `-c sandbox_workspace_write.network_access=true` as extra arguments (extra_args in delegate, after -- on the command line).",
	},
	args:  codexArgs,
	parse: parseCodex,
}

// exec resume takes no --sandbox or --cd, so the policy goes through -c and
// the directory through the process itself. The prompt arrives on stdin.
func codexArgs(req Request, _ string) []string {
	args := []string{"exec"}
	if req.Session != "" {
		args = append(args, "resume")
	}
	args = append(args, "--json", "--skip-git-repo-check",
		"-c", "sandbox_mode="+strconv.Quote(req.Sandbox),
		"-c", `approval_policy="never"`)
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Reasoning != "" {
		args = append(args, "-c", "model_reasoning_effort="+strconv.Quote(req.Reasoning))
	}
	args = append(args, req.ExtraArgs...)
	if req.Session != "" {
		args = append(args, req.Session)
	}
	return append(args, "-")
}

func parseCodex(t *Tracker, line []byte) {
	var e struct {
		Type     string          `json:"type"`
		ThreadID string          `json:"thread_id"`
		Message  json.RawMessage `json:"message"`
		Error    json.RawMessage `json:"error"`
		Usage    json.RawMessage `json:"usage"`
		Item     struct {
			Type     string          `json:"type"`
			Status   string          `json:"status"`
			Text     string          `json:"text"`
			Message  json.RawMessage `json:"message"`
			Command  string          `json:"command"`
			ExitCode *int            `json:"exit_code"`
			Server   string          `json:"server"`
			Tool     string          `json:"tool"`
			Changes  []struct {
				Path string `json:"path"`
				Kind string `json:"kind"`
			} `json:"changes"`
		} `json:"item"`
	}
	if json.Unmarshal(line, &e) != nil {
		return
	}
	if e.ThreadID != "" {
		t.session = e.ThreadID
	}
	item := e.Item
	switch e.Type {
	case "item.started":
		if item.Type == "command_execution" {
			t.note("$ " + item.Command)
		}
	case "item.completed":
		switch item.Type {
		case "agent_message":
			t.say(item.Text)
		case "command_execution":
			t.command(item.Command, item.ExitCode)
		case "file_change":
			if item.Status == "failed" {
				for _, c := range item.Changes {
					t.note(c.Kind + " " + t.rel(c.Path) + " failed")
				}
				break
			}
			// An empty status is the older format, which only reports
			// finished changes.
			for _, c := range item.Changes {
				t.file(c.Path)
				t.note(c.Kind + " " + t.rel(c.Path))
			}
		case "mcp_tool_call":
			t.note("tool " + item.Server + "/" + item.Tool)
		case "error":
			t.note("error: " + textOf(item.Message))
		}
	case "turn.completed":
		t.markDone()
		t.usage = e.Usage
	case "turn.failed":
		t.turnFailed = FirstNonEmpty(textOf(e.Error), "Codex reported a failed turn")
	case "error":
		t.lastError = FirstNonEmpty(textOf(e.Message), "Codex reported an error")
		t.note("error: " + t.lastError)
	}
}
