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

// How each planner loads `harness-bridge mcp` for all projects, and how
// `harness-bridge disconnect` undoes it. Muse Code has no command for it and
// its settings key is ambiguous, so it is not offered as a planner (setup --dir
// still writes it an AGENTS.md).

// Attached here rather than in the adapter literals: some hooks write
// instructions that list the adapters, which would be an initialization cycle.
func init() {
	claudeAdapter.Planner = claudePlanner
	codexAdapter.Planner = codexPlanner
	opencodeAdapter.Planner = opencodePlanner
	commandCodeAdapter.Planner = commandCodePlanner
}

// Claude Code defers MCP tools until first use and treats a server's
// instructions as a soft hint, so the instructions also go into its global
// CLAUDE.md, which it reads in every project.
var claudePlanner = &plannerHooks{
	register: func(cli, exe string, cfg Config) error {
		// Adding a name that exists fails, so replace any earlier registration.
		_ = runQuiet(time.Minute, cli, "mcp", "remove", mcpServerName, "--scope", "user")
		if err := runQuiet(time.Minute, cli, "mcp", "add", "--scope", "user", mcpServerName, "--", exe, "mcp"); err != nil {
			return err
		}
		return writeClaudeInstructions(cfg)
	},
	unregister: func(cli string) error {
		// Try every step, so a failed one doesn't leave the others connected.
		var first error
		if cli != "" {
			if err := runQuiet(time.Minute, cli, "mcp", "remove", mcpServerName, "--scope", "user"); err != nil && !alreadyGone(err) && first == nil {
				first = err
			}
		}
		if err := removeClaudeAllow(claudeSettingsPath(), "mcp__"+mcpServerName); err != nil && first == nil {
			first = err
		}
		if _, err := removeBlock(claudeInstructionsPath()); err != nil && first == nil {
			first = err
		}
		return first
	},
	connectNote: "Connect harness-bridge to Claude Code for all your projects, with its instructions in ~/.claude/CLAUDE.md",
	allowNote:   "Let Claude Code use harness-bridge's tools without asking each time",
	allow: func() error {
		return addClaudePermissions(claudeSettingsPath(), []string{"mcp__" + mcpServerName}, nil)
	},
}

func claudeConfigDir() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func claudeSettingsPath() string {
	return filepath.Join(claudeConfigDir(), "settings.json")
}

func claudeInstructionsPath() string {
	return filepath.Join(claudeConfigDir(), "CLAUDE.md")
}

// writeClaudeInstructions adds the Claude-specific line about deferred tools to
// the instructions that go into its global CLAUDE.md.
func writeClaudeInstructions(cfg Config) error {
	if err := os.MkdirAll(claudeConfigDir(), 0o755); err != nil {
		return err
	}
	body := mcpInstructions(cfg) + "\n" +
		"In Claude Code these are the tools mcp__harness-bridge__delegate and mcp__harness-bridge__wait (and route, if routing is on). If they are deferred, load them with ToolSearch before the first call.\n"
	return upsertBlock(claudeInstructionsPath(), body)
}

// Codex replaces an existing entry itself, removes a missing one without
// complaint and reads the server instructions.
var codexPlanner = &plannerHooks{
	register: func(cli, exe string, _ Config) error {
		return runQuiet(time.Minute, cli, "mcp", "add", mcpServerName, "--", exe, "mcp")
	},
	unregister: func(cli string) error {
		if cli == "" {
			return nil
		}
		return runQuiet(time.Minute, cli, "mcp", "remove", mcpServerName)
	},
}

// OpenCode v2 has `mcp add --global`; v1 can only add servers interactively,
// so its global opencode.json is edited instead. Only v2 is documented to read
// server instructions, so they also go into its global AGENTS.md, which both
// versions read.
var opencodePlanner = &plannerHooks{
	register: func(cli, exe string, cfg Config) error {
		if majorVersion(cli) >= 2 {
			if err := runQuiet(time.Minute, cli, "mcp", "add", mcpServerName, "--global", "--", exe, "mcp"); err != nil {
				return err
			}
		} else {
			var entry map[string]any
			_ = json.Unmarshal([]byte(opencodeEntry(exe)), &entry)
			err := editOpenCodeSettings(func(settings map[string]any) bool {
				servers, _ := settings["mcp"].(map[string]any)
				if servers == nil {
					servers = map[string]any{}
				}
				servers[mcpServerName] = entry
				settings["mcp"] = servers
				return true
			})
			if err != nil {
				return fmt.Errorf("%w; add this under \"mcp\" yourself: %s", err, opencodeEntry(exe))
			}
		}
		return writeInstructions(filepath.Join(opencodeConfigDir(), "AGENTS.md"), cfg)
	},
	// v1 keeps servers under mcp, v2 under mcp.servers; remove either.
	unregister: func(string) error {
		dir := opencodeConfigDir()
		var first error
		switch {
		case fileExists(filepath.Join(dir, "opencode.json")):
			err := editOpenCodeSettings(func(settings map[string]any) bool {
				servers, _ := settings["mcp"].(map[string]any)
				if servers == nil {
					return false
				}
				_, v1 := servers[mcpServerName]
				delete(servers, mcpServerName)
				nested, _ := servers["servers"].(map[string]any)
				_, v2 := nested[mcpServerName]
				delete(nested, mcpServerName)
				return v1 || v2
			})
			if err != nil {
				first = fmt.Errorf("%w; remove the %q entry under \"mcp\" yourself", err, mcpServerName)
			}
		case fileExists(filepath.Join(dir, "opencode.jsonc")):
			// Nothing to remove in a comment-carrying file is fine; an entry
			// still there is the user's to take out.
			if data, err := os.ReadFile(filepath.Join(dir, "opencode.jsonc")); err == nil && strings.Contains(string(data), mcpServerName) {
				first = fmt.Errorf("OpenCode's settings are in %s, which harness-bridge can't edit without losing its comments; remove the %q entry yourself", filepath.Join(dir, "opencode.jsonc"), mcpServerName)
			}
		}
		if _, err := removeBlock(filepath.Join(dir, "AGENTS.md")); err != nil && first == nil {
			first = err
		}
		return first
	},
	connectNote: "Connect harness-bridge to OpenCode for all your projects, with its instructions in ~/.config/opencode/AGENTS.md",
}

// opencodeEntry is the v1 server entry. The timeout leaves room for a wait
// call, whichever of OpenCode's documented defaults applies.
func opencodeEntry(exe string) string {
	data, _ := json.Marshal(map[string]any{"type": "local", "command": []string{exe, "mcp"}, "enabled": true, "timeout": 120000})
	return string(data)
}

// editOpenCodeSettings applies change to the global opencode.json and writes it
// back if change reports a change. JSON with comments (opencode.jsonc, or a
// .json file with comments) is refused rather than stripped.
func editOpenCodeSettings(change func(map[string]any) bool) error {
	dir := opencodeConfigDir()
	path := filepath.Join(dir, "opencode.json")
	if !fileExists(path) && fileExists(filepath.Join(dir, "opencode.jsonc")) {
		return fmt.Errorf("OpenCode's settings are in %s, which harness-bridge can't edit without losing its comments", filepath.Join(dir, "opencode.jsonc"))
	}
	settings := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("%s is not plain JSON, so it was left unchanged", path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !change(settings) {
		return nil
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
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
		return writeInstructions(commandCodeInstructionsPath(), cfg)
	},
	unregister: func(cli string) error {
		// Try every step, so a failed one doesn't leave the block behind.
		var first error
		if cli != "" {
			if err := runQuiet(time.Minute, cli, "mcp", "remove", "--scope", "user", mcpServerName); err != nil && !alreadyGone(err) && first == nil {
				first = err
			}
		}
		if _, err := removeBlock(commandCodeInstructionsPath()); err != nil && first == nil {
			first = err
		}
		return first
	},
	connectNote: "Connect harness-bridge to Command Code for all your projects, with its instructions in ~/.commandcode/AGENTS.md",
}

func commandCodeInstructionsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".commandcode", "AGENTS.md")
}

// writeInstructions puts the server instructions into a global instructions
// file, inside the block that rerunning setup replaces and disconnect removes.
func writeInstructions(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return upsertBlock(path, mcpInstructions(cfg))
}

// alreadyGone reports a removal that failed only because nothing was there.
func alreadyGone(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") || strings.Contains(msg, "no mcp server") || strings.Contains(msg, "does not exist")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
