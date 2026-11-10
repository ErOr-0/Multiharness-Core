package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var museAdapter = &adapter{
	Name: "muse", Title: "Muse Code", Bin: "muse",
	PromptFile:   true,
	Instructions: "AGENTS.md",
	Login:        "muse login",
	Docs:         "https://dev.meta.ai/docs/muse-code",
	Install: []installer{
		{GOOS: "windows", Needs: "powershell", Command: []string{"powershell", "-NoProfile", "-Command", "irm https://dev.meta.ai/install.ps1 | iex"}},
		{GOOS: "!windows", Needs: "sh", Command: []string{"sh", "-c", "curl -fsSL https://dev.meta.ai/install.sh | sh"}},
	},
	Notes: []string{
		"Muse Code cannot continue a session, so send fixes as a new job that repeats the context it needs.",
		"Muse Code runs without a shell here, so it cannot run checks; run them yourself during review.",
	},
	args:    museArgs,
	parse:   parseMuse,
	resolve: museExecutable,
}

// Defaults match what Multiharness shipped with; --model and --reasoning
// override them.
const (
	museDefaultModel     = "muse-spark-1.3"
	museDefaultReasoning = "medium"
)

// Headless Muse cannot ask before shell commands, so the shell stays off; file
// edits inside the workspace need no confirmation under the ask-me profile.
func museArgs(req Request, promptPath string) []string {
	profile := ":ask-me"
	if req.Sandbox == "read-only" {
		profile = ":read-only"
	}
	args := []string{"exec", "--json", "--provider", "meta", "--workspace", req.Dir,
		"--model", firstNonEmpty(req.Model, museDefaultModel),
		"--reasoning-effort", firstNonEmpty(req.Reasoning, museDefaultReasoning),
		"--permission-profile", profile, "--disable-shell",
		"--no-foreign-personal-context", "--no-session-log", "--prompt-file", promptPath}
	if req.Sandbox == "read-only" {
		args = append(args, "--disable-write")
	}
	return append(args, req.ExtraArgs...)
}

// parseMuse reads Muse's versioned event envelopes. Only the session stream's
// root run decides the outcome; nested runs are ignored.
func parseMuse(t *tracker, line []byte) {
	var e struct {
		Version int    `json:"schema_version"`
		Type    string `json:"payload_type"`
		Stream  struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"stream"`
		Payload json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(line, &e) != nil || e.Version != 1 || e.Stream.Kind != "session" {
		return
	}
	var p struct {
		Command  string `json:"command_id"`
		Terminal string `json:"terminal"`
		Text     string `json:"text"`
		Reason   string `json:"reason"`
	}
	_ = json.Unmarshal(e.Payload, &p)
	switch {
	case e.Type == "run.lifecycle.started":
		if t.rootRun == "" && p.Command != "" {
			t.rootRun, t.session = p.Command, e.Stream.ID
			t.note("started")
		}
	case strings.HasPrefix(e.Type, "run.terminal."):
		if p.Command == "" || p.Command != t.rootRun {
			return
		}
		if e.Type == "run.terminal.completed" && p.Terminal == "completed" && p.Reason == "" {
			t.say(p.Text)
			t.markDone()
		} else {
			t.turnFailed = firstNonEmpty(p.Reason, "Muse Code ended with "+strings.TrimPrefix(e.Type, "run.terminal."))
		}
	}
}

var museVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+-R[0-9]+(\.[0-9]+)?$`)

// museExecutable finds the native binary behind the official Windows launcher,
// so no argument passes through cmd.exe.
func museExecutable(name string) (string, error) {
	if runtime.GOOS != "windows" {
		return name, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("Muse Code is not installed; run `harness-bridge setup --implementer muse`: %w", err)
	}
	if strings.EqualFold(filepath.Ext(path), ".exe") {
		return path, nil
	}
	if !strings.EqualFold(filepath.Base(path), "muse.cmd") {
		return "", errors.New("unsupported Muse Code launcher; pass the native executable with --bin")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(path), ".muse-version"))
	if err != nil {
		return "", fmt.Errorf("read the installed Muse Code version: %w", err)
	}
	version := strings.TrimSpace(string(data))
	if !museVersion.MatchString(version) {
		return "", errors.New("the installed Muse Code version is invalid; reinstall Muse Code")
	}
	native := filepath.Join(filepath.Dir(path), "muse-bin-"+version+".exe")
	if info, err := os.Stat(native); err != nil || !info.Mode().IsRegular() {
		return "", errors.New("the Muse Code native executable is missing; reinstall Muse Code")
	}
	return native, nil
}
