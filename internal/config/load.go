package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxConfigBytes = 1 << 20

// Load reads an explicitly selected file (if any). Empty environment values
// are real overrides, not an instruction to fall back to a lower layer.
func Load(filename, baseDir string, lookupEnv func(string) (string, bool), overrides map[string]string) (Config, error) {
	if err := rejectRemovedPlannerEnvironment(lookupEnv); err != nil {
		return Config{}, err
	}
	c := Defaults()
	supplied := map[string]bool{}
	if filename != "" {
		if !filepath.IsAbs(filename) {
			filename = filepath.Join(baseDir, filename)
		}
		data, err := ReadFile(filename, MaxConfigBytes)
		if err != nil {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
		if err := decodeStrict(data, &c); err != nil {
			return Config{}, fmt.Errorf("config file: %w", err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil || fields["version"] == nil {
			return Config{}, fmt.Errorf("config file must declare version 1")
		}
		markPlannerFields(fields["reviewer"], "reviewer.", supplied)
		markPlannerFields(fields["planner"], "planner.", supplied)
		markPlannerFields(fields["implementer"], "implementer.", supplied)
		var fallback map[string]json.RawMessage
		_ = json.Unmarshal(fields["fallback"], &fallback)
		markPlannerFields(fallback["planner"], "fallback.planner.", supplied)
		if c.Version != 1 {
			return Config{}, fmt.Errorf("unsupported configuration version (expected 1)")
		}
	}
	known := map[string]bool{}
	for _, option := range Options() {
		known[option.Name] = true
	}
	// Select winning values before decoding: an overridden invalid lower-layer
	// value must not reject a valid explicit flag, including through legacy aliases.
	type selectedOption struct {
		option Option
		value  string
	}
	selected := map[string]selectedOption{}
	for _, environment := range []bool{true, false} {
		seen := map[string]string{}
		for _, option := range Options() {
			var value string
			var provided bool
			if environment {
				if lookupEnv != nil {
					value, provided = lookupEnv(option.Environment())
				}
			} else {
				value, provided = overrides[option.Name]
			}
			if !provided {
				continue
			}
			if previous, exists := seen[option.Path]; exists && previous != value {
				return Config{}, fmt.Errorf("conflicting aliases for %s", option.Path)
			}
			seen[option.Path] = value
			selected[option.Path] = selectedOption{option, value}
		}
	}
	for _, option := range Options() {
		winner, exists := selected[option.Path]
		if !exists || winner.option.Name != option.Name {
			continue
		}
		supplied[option.Path] = true
		if err := apply(&c, option, winner.value); err != nil {
			return Config{}, fmt.Errorf("setting %s: %w", option.Name, err)
		}
	}
	for name := range overrides {
		if !known[name] {
			return Config{}, fmt.Errorf("unknown setting %q", name)
		}
	}
	c.Planner.resolveDefaults("planner.", supplied)
	c.Implementer.resolveDefaults(supplied)
	c.Reviewer.resolveDefaults("reviewer.", supplied)
	if !supplied["fallback.planner.harness"] {
		c.Fallback.Planner.Harness = "opencode"
		if c.Planner.Harness == "opencode" {
			c.Fallback.Planner.Harness = "codex"
		}
	}
	c.Fallback.Planner.resolveDefaults("fallback.planner.", supplied)
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	if !filepath.IsAbs(baseDir) {
		return Config{}, fmt.Errorf("configuration base directory must be absolute")
	}
	c.ResolvePaths(baseDir)
	return c, nil
}

// Strict decoding has already checked the types; this records presence so an
// explicit empty value remains distinct from a provider-dependent default.
func markPlannerFields(data json.RawMessage, prefix string, supplied map[string]bool) {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	for key := range fields {
		supplied[prefix+key] = true
	}
}

func rejectRemovedPlannerEnvironment(lookup func(string) (string, bool)) error {
	if lookup == nil {
		return nil
	}
	for old, current := range map[string]string{
		"MULTIHARNESS_OPENCODE_PLANNER_":          "MULTIHARNESS_PLANNER_",
		"MULTIHARNESS_FALLBACK_OPENCODE_PLANNER_": "MULTIHARNESS_FALLBACK_PLANNER_",
	} {
		for _, field := range []string{"EXECUTABLE", "MODEL", "VARIANT", "TIMEOUT", "PERMISSION_POLICY", "EXTRA_ARGS"} {
			if _, present := lookup(old + field); present {
				return fmt.Errorf("%s was removed; use %s and select the planner harness", old+field, current+field)
			}
		}
	}
	return nil
}

func apply(c *Config, option Option, value string) error {
	var raw json.RawMessage
	if option.JSON {
		raw = json.RawMessage(value)
	} else {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		raw = encoded
	}
	var node any = raw
	parts := strings.Split(option.Path, ".")
	for i := len(parts) - 1; i >= 0; i-- {
		node = map[string]any{parts[i]: node}
	}
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("expected a valid JSON value")
	}
	return decodeStrict(data, c)
}

// ReadFile bounds config/task input and rejects directories and named pipes.
func ReadFile(filename string, limit int) ([]byte, error) {
	if limit <= 0 || int64(limit) == math.MaxInt64 {
		return nil, fmt.Errorf("input byte limit must be positive")
	}
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input must be a regular file")
	}
	if info.Size() > int64(limit) {
		return nil, fmt.Errorf("input exceeds %d bytes", limit)
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("input exceeds %d bytes", limit)
	}
	return data, nil
}

func decodeStrict(data []byte, target any) error {
	if !utf8.Valid(data) {
		return fmt.Errorf("configuration must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := checkJSON(decoder, false); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("expected one JSON document")
	}
	// Accept the former version-1 git settings as a migration alias. Never
	// merge two independently supplied workspace sections or execute the old tool.
	if _, ok := target.(*Config); ok {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return err
		}
		if legacy, exists := fields["git"]; exists {
			if _, duplicate := fields["workspace"]; duplicate {
				return fmt.Errorf("use workspace or legacy git settings, not both")
			}
			fields["workspace"] = legacy
			delete(fields, "git")
			var err error
			data, err = json.Marshal(fields)
			if err != nil {
				return err
			}
		}
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// Reject ambiguous keys and null: encoding/json otherwise matches fields without
// regard to case, overwrites duplicates, or treats null as a merge no-op. Only
// environment-variable names are data keys and may retain their original case.
func checkJSON(decoder *json.Decoder, environmentKeys bool) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return fmt.Errorf("null configuration values are not allowed")
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			canonical := strings.ToLower(name)
			if !ok || seen[canonical] {
				return fmt.Errorf("invalid or duplicate JSON key")
			}
			if !environmentKeys && name != canonical {
				return fmt.Errorf("configuration property names must be lowercase")
			}
			seen[canonical] = true
			if err := checkJSON(decoder, name == "env_overrides"); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	case json.Delim('['):
		for decoder.More() {
			if err := checkJSON(decoder, false); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
		return err
	}
	return nil
}

// ResolvePaths anchors application paths to the invocation directory, not the
// config file or an agent-controlled checkout. Validation scripts are the one
// deliberate exception: explicit relative paths are anchored to the target.
func (c *Config) ResolvePaths(baseDir string) {
	if c.Workspace.RecoveryDir != "" && !filepath.IsAbs(c.Workspace.RecoveryDir) {
		c.Workspace.RecoveryDir = filepath.Join(baseDir, c.Workspace.RecoveryDir)
	}
	if !filepath.IsAbs(c.WorkingDir) {
		c.WorkingDir = filepath.Join(baseDir, c.WorkingDir)
	}
	for _, command := range []*string{
		&c.Planner.Executable,
		&c.Reviewer.Executable,
		&c.Implementer.Executable,
		&c.Fallback.CodexImplementer.Executable,
		&c.Fallback.Planner.Executable,
		&c.Fallback.OpenCodeReviewer.Executable,
	} {
		*command = resolveCommand(baseDir, *command)
	}
	for i := range c.Validation.Checks {
		c.Validation.Checks[i].Executable = resolveCommand(c.WorkingDir, c.Validation.Checks[i].Executable)
	}
}

func resolveCommand(baseDir, command string) string {
	if !filepath.IsAbs(command) && strings.ContainsAny(command, `/\`) {
		return filepath.Join(baseDir, command)
	}
	return command
}
