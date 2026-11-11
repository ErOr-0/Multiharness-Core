package agent

import (
	"encoding/json"
	"slices"
	"strings"
)

// yoloRequired is how Command Code 1.88 refuses a file edit in print mode,
// even under accept-edits: only --yolo, which also allows any shell command,
// lets a headless run write.
const yoloRequired = "requires permission"

const commandCodeYoloHint = "Command Code refused to edit files: when it runs headless it edits only with --yolo, which also lets it run any shell command. Ask the user before sending the job again with extra_args [\"--yolo\"]."

var CommandCode = &Adapter{
	Name: "commandcode", Title: "Command Code", Bin: "command-code",
	Resume:       true,
	Instructions: "AGENTS.md",
	Login:        "command-code login",
	LoginCheck:   []string{"status", "--json"},
	Docs:         "https://commandcode.ai/docs/quickstart",
	ModelList:    []string{"--list-models"},
	Efforts:      []string{"low", "medium", "high"},
	Install: []Installer{
		{Needs: "npm", Command: []string{"npm", "install", "--global", "command-code@latest"}},
	},
	Notes: []string{
		"Command Code 1.88 edits files headless only with --yolo, which also lets it run any shell command. Without it a job ends `needs_permission`; ask the user before passing `--yolo` in extra_args (after -- on the command line), and only for that job.",
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
func parseCommandCode(t *Tracker, line []byte) {
	var e struct {
		Type       string          `json:"type"`
		Subtype    string          `json:"subtype"`
		SessionID  string          `json:"sessionId"`
		Usage      json.RawMessage `json:"usage"`
		FinalText  string          `json:"finalText"`
		StopReason string          `json:"stopReason"`
		Error      json.RawMessage `json:"error"`
		Event      struct {
			Type        string `json:"type"`
			ToolName    string `json:"toolName"`
			Description string `json:"description"`
			HookOutput  string `json:"hookOutput"`
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
		switch e.Event.Type {
		// Only a real refusal event counts: later events repeat the whole
		// conversation, so the refusal's wording shows up again in runs that
		// were refused earlier or merely mention it.
		case "tool_hook_blocked":
			name := FirstNonEmpty(e.Event.ToolName, "a tool call")
			hook := strings.ToLower(e.Event.HookOutput)
			denied := name
			if strings.Contains(hook, "--yolo") || strings.Contains(hook, yoloRequired) {
				denied = name + " (needs --yolo)"
				t.needsPermission = true
				t.turnFailed = commandCodeYoloHint
			} else {
				t.needsPermission = true
				t.turnFailed = "Command Code blocked " + name + ": " + Clip(e.Event.HookOutput, 200)
			}
			if !slices.Contains(t.denied, denied) {
				t.denied = append(t.denied, denied)
			}
		}
		if e.Event.ToolName != "" {
			t.note(e.Event.ToolName + " " + e.Event.Description)
		}
	case "result":
		t.say(e.FinalText)
		t.usage = e.Usage
		switch e.Subtype {
		case "success":
			// A turn that stopped on a refused action still reports success.
			if e.StopReason == "permission_denied" {
				t.needsPermission = true
				if t.turnFailed == "" {
					t.turnFailed = commandCodeExits[4]
				}
			}
			t.markDone()
		case "max_turns":
			t.turnFailed = "Command Code reached its turn limit; raise it with `-- --max-turns N`"
		default:
			t.lastError = textOf(e.Error)
			if !t.needsPermission {
				t.turnFailed = FirstNonEmpty(t.lastError, "Command Code ended with an error")
			}
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

func finishCommandCode(t *Tracker, code int) {
	hint, ok := commandCodeExits[code]
	if !ok || t.turnFailed == commandCodeYoloHint {
		return
	}
	if t.lastError != "" && t.lastError != hint {
		hint += ": " + t.lastError
	}
	t.turnFailed, t.done = hint, false
	t.needsPermission = code == 4
}
