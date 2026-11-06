package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// How each planner loads `harness-bridge mcp` for all projects. Muse Code has
// no command for it and its settings key is ambiguous, so it is not offered as
// a planner (setup --dir still writes it an AGENTS.md).

// Attached here rather than in the adapter literals: Command Code's hook writes
// instructions that list the adapters, which would be an initialization cycle.
func init() {
	claudeAdapter.Planner = claudePlanner
	codexAdapter.Planner = codexPlanner
	opencodeAdapter.Planner = opencodePlanner
	commandCodeAdapter.Planner = commandCodePlanner
}

var claudePlanner = &plannerHooks{
	register: func(cli, exe string, _ Config) error {
		// Adding a name that exists fails, so replace any earlier registration.
		_ = runQuiet(time.Minute, cli, "mcp", "remove", mcpServerName, "--scope", "user")
		return runQuiet(time.Minute, cli, "mcp", "add", "--scope", "user", mcpServerName, "--", exe, "mcp")
	},
	allowNote: "Let Claude Code use harness-bridge's tools without asking each time",
	allow: func() error {
		return addClaudePermissions(claudeSettingsPath(), []string{"mcp__" + mcpServerName}, nil)
	},
}

func claudeSettingsPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

// Codex replaces an existing entry itself and reads the server instructions.
var codexPlanner = &plannerHooks{
	register: func(cli, exe string, _ Config) error {
		return runQuiet(time.Minute, cli, "mcp", "add", mcpServerName, "--", exe, "mcp")
	},
}

// OpenCode v2 has `mcp add --global`; v1 can only add servers interactively,
// so its global opencode.json is edited instead.
var opencodePlanner = &plannerHooks{
	register: func(cli, exe string, _ Config) error {
		if majorVersion(cli) >= 2 {
			return runQuiet(time.Minute, cli, "mcp", "add", mcpServerName, "--global", "--", exe, "mcp")
		}
		dir := opencodeConfigDir()
		path := filepath.Join(dir, "opencode.json")
		if !fileExists(path) && fileExists(filepath.Join(dir, "opencode.jsonc")) {
			return fmt.Errorf("OpenCode's settings are in opencode.jsonc, which harness-bridge can't edit without losing its comments; add this under \"mcp\" there: %s", opencodeEntry(exe))
		}
		settings := map[string]any{}
		if data, err := os.ReadFile(path); err == nil {
			if err := json.Unmarshal(data, &settings); err != nil {
				return fmt.Errorf("%s is not plain JSON, so it was left unchanged; add this under \"mcp\": %s", path, opencodeEntry(exe))
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		servers, _ := settings["mcp"].(map[string]any)
		if servers == nil {
			servers = map[string]any{}
		}
		var entry map[string]any
		_ = json.Unmarshal([]byte(opencodeEntry(exe)), &entry)
		servers[mcpServerName] = entry
		settings["mcp"] = servers
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, append(data, '\n'), 0o644)
	},
}

// opencodeEntry is the v1 server entry. The timeout leaves room for a wait
// call, whichever of OpenCode's documented defaults applies.
func opencodeEntry(exe string) string {
	data, _ := json.Marshal(map[string]any{"type": "local", "command": []string{exe, "mcp"}, "enabled": true, "timeout": 120000})
	return string(data)
}

func opencodeConfigDir() string {
	if dir := os.Getenv("OPENCODE_CONFIG_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opencode")
}

// majorVersion reads the leading number of `cli --version`, or 0.
func majorVersion(cli string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, cli, "--version").Output()
	if err != nil {
		return 0
	}
	field := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	major, _, _ := strings.Cut(field, ".")
	n, _ := strconv.Atoi(major)
	return n
}

// Command Code ignores MCP server instructions, so they also go into its
// global AGENTS.md, which it reads in every project.
var commandCodePlanner = &plannerHooks{
	register: func(cli, exe string, cfg Config) error {
		_ = runQuiet(time.Minute, cli, "mcp", "remove", "--scope", "user", mcpServerName)
		if err := runQuiet(time.Minute, cli, "mcp", "add", "--scope", "user", mcpServerName, "--", exe, "mcp"); err != nil {
			return err
		}
		path := commandCodeInstructionsPath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return upsertBlock(path, mcpInstructions(cfg))
	},
	connectNote: "Connect harness-bridge to Command Code for all your projects, with its instructions in ~/.commandcode/AGENTS.md",
}

func commandCodeInstructionsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".commandcode", "AGENTS.md")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
