package cli

import (
	"multiharness-core/internal/config"
	"strings"
)

// CommandSuggestions contains only application-owned commands and values, never
// task history, paths, account identifiers or API keys.
func CommandSuggestions(line string) []string {
	if !strings.HasPrefix(line, "/") {
		return nil
	}
	candidates := []string{"/configuration", "/setup", "/config", "/plan", "/plans", "/use", "/history", "/login codex", "/login opencode", "/login claude", "/login jev", "/workspace", "/settings", "/permissions", "/new", "/save", "/load", "/diagnostics", "/options", "/help", "/quit", "/exit", "/set"}
	if strings.HasPrefix(strings.ToLower(line), "/set ") {
		candidates = nil
		for _, opt := range config.Options() {
			candidates = append(candidates, "/set "+opt.Name)
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 && (len(fields) > 2 || strings.HasSuffix(line, " ")) {
			values := []string{}
			switch name := fields[1]; {
			case name == "mode":
				values = []string{"direct", "team"}
			case strings.HasSuffix(name, "-harness"):
				values = []string{"codex", "opencode", "claude"}
			case name == "decision-enabled":
				values = []string{"true", "false"}
			case name == "fallback-mode":
				values = []string{"disabled", "prompt"}
			case name == "progress":
				values = []string{"auto", "plain", "expanded", "off"}
			case name == "color":
				values = []string{"auto", "always", "never"}
			}
			candidates = nil
			for _, value := range values {
				candidates = append(candidates, "/set "+fields[1]+" "+value)
			}
		}
	}
	for _, candidate := range candidates {
		if candidate == strings.ToLower(line) {
			return nil
		}
	}
	var matches []string
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, strings.ToLower(line)) && candidate != strings.ToLower(line) {
			matches = append(matches, candidate)
		}
	}
	return matches
}
