package cli

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"multiharness-core/internal/config"
)

func splitInteractiveWord(value string) (string, string) {
	value = strings.TrimSpace(value)
	if i := strings.IndexFunc(value, unicode.IsSpace); i >= 0 {
		return value[:i], strings.TrimSpace(value[i:])
	}
	return value, ""
}

// Tolerance is limited to documented syntax. Model IDs, executable paths,
// JSON contents and task text never go through fuzzy correction.
func interactiveSetting(value string) (config.Option, string, error) {
	name, setting := splitInteractiveWord(value)
	if before, after, ok := strings.Cut(name, "="); ok {
		name, setting = before, after+" "+setting
	} else {
		setting = strings.TrimSpace(strings.TrimPrefix(setting, "="))
	}
	name = strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(name, "--"), "_", "-"))
	options := config.Options()
	var names []string
	for _, option := range options {
		names = append(names, option.Name)
		if option.Name == name {
			if strings.TrimSpace(setting) == "" {
				return option, "", fmt.Errorf("use /set %s VALUE; use \"\" to clear an optional value\n%s", name, option.Help)
			}
			normalized, err := normalizeSetting(option, setting)
			return option, normalized, err
		}
	}
	return config.Option{}, "", fmt.Errorf("unknown setting %q.%s Use /options to see valid names", terminalText(name), spellingSuggestion(name, names))
}

func normalizeSetting(option config.Option, value string) (string, error) {
	if !utf8.ValidString(value) || strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r'
	}) >= 0 {
		return "", fmt.Errorf("%s contains invalid text or control characters; please retype it", option.Name)
	}
	value = strings.TrimSpace(value)
	// Strip a complete pair of pasted/shell-style outer quotes only. Interior
	// quotes, backslashes, Unicode, spaces and shell-looking text remain literal.
	for _, pair := range [][2]string{{"\"", "\""}, {"'", "'"}, {"“", "”"}, {"‘", "’"}} {
		if strings.HasPrefix(value, pair[0]) {
			if len(value) < len(pair[0])+len(pair[1]) || !strings.HasSuffix(value, pair[1]) {
				return "", fmt.Errorf("%s has an unfinished quote; close it or remove the opening quote", option.Name)
			}
			value = strings.TrimSuffix(strings.TrimPrefix(value, pair[0]), pair[1])
			break
		}
	}
	if strings.HasSuffix(option.Name, "-model") {
		value = strings.TrimSpace(value)
		if strings.IndexFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.In(r, unicode.Cf) }) >= 0 || strings.ContainsAny(value, "\"'“”‘’") {
			return "", fmt.Errorf("%s needs one model ID without spaces, hidden formatting or embedded quotes; for OpenCode use provider/model", option.Name)
		}
	}
	if strings.HasSuffix(option.Name, "-harness") || option.Name == "color" || option.Name == "progress" || option.Name == "log-format" || option.Name == "existing-work" || strings.HasSuffix(option.Name, "-mode") || strings.HasSuffix(option.Name, "-reasoning") || strings.HasSuffix(option.Name, "-sandbox") {
		value = strings.ToLower(strings.TrimSpace(value))
	}
	// Explicit permission tokens are kept exact. A misspelling must not enable
	// broader permissions. Numbers accept leading zeros and an optional plus.
	if option.JSON && !strings.HasSuffix(option.Name, "-args") && option.Name != "validation-checks" {
		if number, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			value = strconv.FormatInt(number, 10)
		}
	}
	return value, nil
}

func spellingSuggestion(value string, candidates []string) string {
	// Bound work on arbitrary input; config/command names are short ASCII tokens.
	if len(value) < 2 || len(value) > 64 {
		return ""
	}
	best, distance := "", 3
	for _, candidate := range candidates {
		d := editDistance(value, candidate)
		if d < distance {
			best, distance = candidate, d
		} else if d == distance {
			best = "" // Ambiguous guesses are not useful corrections.
		}
	}
	if best != "" {
		return fmt.Sprintf(" Did you mean %s?", best)
	}
	return ""
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for i := range previous {
		previous[i] = i
	}
	for i := range len(a) {
		row := make([]int, len(b)+1)
		row[0] = i + 1
		for j := range len(b) {
			cost := 0
			if a[i] != b[j] {
				cost = 1
			}
			row[j+1] = min(row[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = row
	}
	return previous[len(b)]
}

// Selecting a different provider starts its configuration with matching defaults,
// preventing model IDs, executable pins and extra arguments from crossing CLIs.
// This is scoped to an explicit interactive selection, not file/flag precedence.
func selectInteractivePlanner(overrides map[string]string, cfg config.Config, harness string) {
	if harness == cfg.Planner.Harness || !supportedHarness(harness) {
		return
	}
	alternate := "opencode"
	if harness == "opencode" {
		alternate = "codex"
	}
	for _, role := range []struct{ prefix, harness string }{{"planner-", harness}, {"fallback-planner-", alternate}} {
		defaults := config.DefaultPlanner(role.harness)
		for key, value := range map[string]string{
			"harness": defaults.Harness, "executable": defaults.Executable,
			"model": defaults.Model, "reasoning": defaults.Reasoning, "variant": defaults.Variant,
			"extra-args": "[]", "sandbox": string(defaults.Sandbox), "permission-policy": string(defaults.PermissionPolicy),
		} {
			overrides[role.prefix+key] = value
		}
	}
}

func selectInteractiveImplementer(overrides map[string]string, cfg config.Config, harness string) {
	if harness == cfg.Implementer.Harness || !supportedHarness(harness) {
		return
	}
	defaults := config.DefaultImplementer(harness)
	for key, value := range map[string]string{
		"harness": defaults.Harness, "executable": defaults.Executable,
		"model": defaults.Model, "reasoning": defaults.Reasoning, "variant": defaults.Variant,
		"extra-args": "[]", "sandbox": string(defaults.Sandbox), "permission-policy": string(defaults.PermissionPolicy),
	} {
		overrides["implementer-"+key] = value
	}
}

func supportedHarness(harness string) bool {
	return harness == "codex" || harness == "opencode" || harness == "claude"
}
func selectInteractiveReviewer(overrides map[string]string, cfg config.Config, harness string) {
	if harness == cfg.Reviewer.Harness || !supportedHarness(harness) {
		return
	}
	defaults := config.DefaultPlanner(harness)
	for key, value := range map[string]string{"harness": defaults.Harness, "executable": defaults.Executable, "model": defaults.Model, "reasoning": defaults.Reasoning, "variant": defaults.Variant, "extra-args": "[]", "sandbox": string(defaults.Sandbox), "permission-policy": string(defaults.PermissionPolicy)} {
		overrides["reviewer-"+key] = value
	}
}

// Numeric choices apply only inside the reasoning prompt. Model identifiers and
// OpenCode variants remain literal user input.
func reasoningChoices(harness string) []string {
	choices := []string{"low", "medium", "high", "xhigh", "max"}
	if harness == "codex" {
		choices = append(choices, "none")
	}
	return choices
}
func reasoningSelection(harness, value string) string {
	choices := reasoningChoices(harness)
	if n, err := strconv.Atoi(value); err == nil && n >= 1 && n <= len(choices) {
		return choices[n-1]
	}
	return value
}
