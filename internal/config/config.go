// Package config loads explicit, versioned application settings. Precedence is
// defaults, JSON file, environment, then explicitly supplied CLI overrides.
// No repository configuration is discovered or executed automatically.
package config

import (
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/codex"
	"multiharness-core/internal/adapter/agent/opencode"
	"multiharness-core/internal/adapter/process"
	validationadapter "multiharness-core/internal/adapter/validation"
	gitworkspace "multiharness-core/internal/adapter/workspace/git"
	"multiharness-core/internal/workflow"
)

// Duration uses human-readable Go durations ("30s", "5m", "2h") in JSON.
type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }
func (d *Duration) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("duration must be a string")
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("invalid duration; use units such as 30s or 5m")
	}
	*d = Duration(parsed)
	return nil
}

type Codex struct {
	Executable string            `json:"executable"`
	Model      string            `json:"model"`
	Reasoning  string            `json:"reasoning"`
	Timeout    Duration          `json:"timeout"`
	Sandbox    codex.SandboxMode `json:"sandbox"`
	ExtraArgs  []string          `json:"extra_args"`
}

type OpenCode struct {
	Executable       string                    `json:"executable"`
	Model            string                    `json:"model"`
	Variant          string                    `json:"variant"`
	Timeout          Duration                  `json:"timeout"`
	PermissionPolicy opencode.PermissionPolicy `json:"permission_policy"`
	ExtraArgs        []string                  `json:"extra_args"`
}

type Git struct {
	Executable       string   `json:"executable"`
	Timeout          Duration `json:"timeout"`
	MaxFiles         int      `json:"max_files"`
	MaxFileBytes     int64    `json:"max_file_bytes"`
	MaxSnapshotBytes int64    `json:"max_snapshot_bytes"`
	MaxOutputBytes   int      `json:"max_output_bytes"`
}

type Check struct {
	Executable   string            `json:"executable"`
	Args         []string          `json:"args,omitempty"`
	Timeout      Duration          `json:"timeout"`
	EnvOverrides map[string]string `json:"env_overrides,omitempty"`
}

type Validation struct {
	Checks         []Check  `json:"checks"`
	DefaultTimeout Duration `json:"default_timeout"`
	OutputLimit    int      `json:"output_limit"`
}

type Config struct {
	Version           int        `json:"version"`
	WorkingDir        string     `json:"working_dir"`
	MaxRepairAttempts int        `json:"max_repair_attempts"`
	SessionID         string     `json:"session_id"`
	Timeout           Duration   `json:"timeout"`
	MaxTaskBytes      int        `json:"max_task_bytes"`
	LogFormat         string     `json:"log_format"`
	Color             string     `json:"color"`
	Progress          string     `json:"progress"`
	Planner           Codex      `json:"planner"`
	Reviewer          Codex      `json:"reviewer"`
	Implementer       OpenCode   `json:"implementer"`
	Git               Git        `json:"git"`
	Validation        Validation `json:"validation"`
	Execution         Execution  `json:"execution"`
	Fallback          Fallback   `json:"fallback"`
}

type Fallback struct {
	Mode             string   `json:"mode"`
	CodexImplementer Codex    `json:"codex_implementer"`
	OpenCodePlanner  OpenCode `json:"opencode_planner"`
	OpenCodeReviewer OpenCode `json:"opencode_reviewer"`
}

type Execution struct {
	MaxAgentInvocations int      `json:"max_agent_invocations"`
	MaxRetries          int      `json:"max_retries"`
	InitialDelay        Duration `json:"initial_delay"`
	MaxDelay            Duration `json:"max_delay"`
	// Zero means no monetary cap. Positive requests fail closed because the
	// supported CLI interfaces cannot enforce authoritative per-request spend.
	MaxCostMicrousd int64 `json:"max_cost_microusd"`
}

func (e Execution) Policy() workflow.ExecutionPolicy {
	return workflow.ExecutionPolicy{MaxAgentInvocations: e.MaxAgentInvocations, MaxRetries: e.MaxRetries, InitialDelay: time.Duration(e.InitialDelay), MaxDelay: time.Duration(e.MaxDelay)}
}

func Defaults() Config {
	c := codex.DefaultConfig()
	o := opencode.DefaultConfig()
	g := gitworkspace.DefaultConfig()
	p := workflow.DefaultExecutionPolicy()
	return Config{
		Version: 1, WorkingDir: ".", MaxRepairAttempts: 3, Timeout: Duration(4 * time.Hour), MaxTaskBytes: 1 << 20, LogFormat: "text", Color: "auto", Progress: "auto",
		Planner:     Codex{c.Executable, c.Model, c.Reasoning, Duration(c.Timeout), c.Sandbox, []string{}},
		Reviewer:    Codex{c.Executable, c.Model, c.Reasoning, Duration(c.Timeout), c.Sandbox, []string{}},
		Implementer: OpenCode{o.Executable, o.Model, o.Variant, Duration(o.Timeout), o.PermissionPolicy, []string{}},
		Git:         Git{g.Executable, Duration(g.Timeout), g.MaxFiles, g.MaxFileBytes, g.MaxSnapshotBytes, g.MaxOutputBytes},
		Validation:  Validation{Checks: []Check{}, DefaultTimeout: Duration(5 * time.Minute), OutputLimit: 64 << 10},
		Execution:   Execution{MaxAgentInvocations: p.MaxAgentInvocations, MaxRetries: p.MaxRetries, InitialDelay: Duration(p.InitialDelay), MaxDelay: Duration(p.MaxDelay)},
		Fallback: Fallback{
			Mode:             "prompt",
			CodexImplementer: Codex{c.Executable, c.Model, c.Reasoning, Duration(o.Timeout), codex.SandboxWorkspaceWrite, []string{}},
			OpenCodePlanner:  OpenCode{o.Executable, o.Model, o.Variant, Duration(c.Timeout), opencode.PermissionRejectOnPrompt, []string{}},
			OpenCodeReviewer: OpenCode{o.Executable, o.Model, o.Variant, Duration(c.Timeout), opencode.PermissionRejectOnPrompt, []string{}},
		},
	}
}

// Validate is side-effect free: executable availability and authentication are
// checked by the process adapter when each stage actually invokes its command.
// An answer-only run therefore does not require an installed OpenCode binary.
func (c Config) Validate() error {
	if c.Fallback.Mode != "prompt" && c.Fallback.Mode != "disabled" {
		return fmt.Errorf("fallback.mode must be prompt or disabled; unattended switching is not permitted")
	}
	if err := c.Fallback.CodexImplementer.Adapter().Validate(); err != nil {
		return fmt.Errorf("fallback.codex_implementer: %w", err)
	}
	if err := executable(c.Fallback.CodexImplementer.Executable); err != nil {
		return err
	}
	if c.Fallback.CodexImplementer.Sandbox != codex.SandboxWorkspaceWrite {
		return fmt.Errorf("fallback.codex_implementer.sandbox must be workspace-write")
	}
	model := c.Fallback.CodexImplementer.Model
	if strings.TrimSpace(model) == "" || strings.ContainsAny(model, " \t\r\n\x00") || strings.HasPrefix(model, "-") {
		return fmt.Errorf("fallback.codex_implementer.model must be a model identifier")
	}
	for _, agent := range []OpenCode{c.Fallback.OpenCodePlanner, c.Fallback.OpenCodeReviewer} {
		if err := agent.Adapter().Validate(); err != nil {
			return fmt.Errorf("fallback read-only agent: %w", err)
		}
		if err := executable(agent.Executable); err != nil {
			return err
		}
		if agent.PermissionPolicy != opencode.PermissionRejectOnPrompt {
			return fmt.Errorf("fallback planning/review requires reject_on_prompt")
		}
	}
	if c.Version != 1 {
		return fmt.Errorf("unsupported configuration version (expected 1)")
	}
	if c.SessionID != "" && (strings.ContainsAny(c.SessionID, " \t\r\n\x00") || strings.HasPrefix(c.SessionID, "-")) {
		return fmt.Errorf("session_id must not contain whitespace, control characters, or leading dashes")
	}
	if c.LogFormat != "text" && c.LogFormat != "json" {
		return fmt.Errorf("log_format must be text or json")
	}
	if c.Color != "auto" && c.Color != "always" && c.Color != "never" {
		return fmt.Errorf("color must be auto, always or never")
	}
	if c.Progress != "auto" && c.Progress != "plain" && c.Progress != "off" {
		return fmt.Errorf("progress must be auto, plain or off")
	}
	if err := c.Execution.Policy().Validate(); err != nil {
		return fmt.Errorf("execution: %w", err)
	}
	if c.Execution.MaxCostMicrousd != 0 {
		return fmt.Errorf("execution.max_cost_microusd: monetary caps cannot be enforced by the CLI adapters; use enforced provider billing limits or a metered gateway (0 disables this unsupported cap)")
	}
	if strings.TrimSpace(c.WorkingDir) == "" || strings.ContainsRune(c.WorkingDir, 0) {
		return fmt.Errorf("working_dir must be a nonempty path without NUL")
	}
	if c.MaxRepairAttempts < 0 {
		return fmt.Errorf("max_repair_attempts must be zero or greater")
	}
	if c.Timeout <= 0 || c.MaxTaskBytes <= 0 || int64(c.MaxTaskBytes) == math.MaxInt64 {
		return fmt.Errorf("timeout and max_task_bytes must be positive")
	}
	for _, agent := range []struct {
		name   string
		config Codex
	}{{"planner", c.Planner}, {"reviewer", c.Reviewer}} {
		if err := executable(agent.config.Executable); err != nil {
			return fmt.Errorf("%s.executable: %w", agent.name, err)
		}
		if strings.TrimSpace(agent.config.Model) == "" || strings.ContainsAny(agent.config.Model, " \t\r\n\x00") || strings.HasPrefix(agent.config.Model, "-") {
			return fmt.Errorf("%s.model must be a nonempty model identifier", agent.name)
		}
		if err := agent.config.Adapter().Validate(); err != nil {
			return fmt.Errorf("%s: %w", agent.name, err)
		}
		if agent.config.Sandbox != codex.SandboxReadOnly {
			return fmt.Errorf("%s.sandbox must be read-only", agent.name)
		}
	}
	if err := executable(c.Implementer.Executable); err != nil {
		return fmt.Errorf("implementer.executable: %w", err)
	}
	if err := c.Implementer.Adapter().Validate(); err != nil {
		return fmt.Errorf("implementer: %w", err)
	}
	if err := executable(c.Git.Executable); err != nil {
		return fmt.Errorf("git.executable: %w", err)
	}
	if c.Git.Timeout <= 0 || c.Git.MaxFiles <= 0 || c.Git.MaxFileBytes <= 0 || c.Git.MaxSnapshotBytes <= 0 || c.Git.MaxOutputBytes <= 0 {
		return fmt.Errorf("Git timeout and limits must be positive")
	}
	if _, err := gitworkspace.NewWorkspace(process.NewOSRunner(), c.Git.Adapter()); err != nil {
		return err
	}
	if c.Validation.DefaultTimeout <= 0 || c.Validation.OutputLimit <= 0 {
		return fmt.Errorf("validation timeout and output_limit must be positive")
	}
	for i, check := range c.Validation.Checks {
		if err := executable(check.Executable); err != nil {
			return fmt.Errorf("validation.checks[%d].executable: %w", i, err)
		}
	}
	if _, err := validationadapter.NewValidator(process.NewOSRunner(), c.Validation.Adapter()); err != nil {
		return err
	}
	return nil
}

func executable(value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00") || strings.HasPrefix(value, "-") {
		return fmt.Errorf("must be an executable name or path without surrounding whitespace, NUL, or a flag prefix")
	}
	return nil
}

// ResolvePaths anchors application paths to the invocation directory, not the
// config file or an agent-controlled checkout. Validation scripts are the one
// deliberate exception: explicit relative paths are anchored to the target.
func (c *Config) ResolvePaths(baseDir string) {
	if !filepath.IsAbs(c.WorkingDir) {
		c.WorkingDir = filepath.Join(baseDir, c.WorkingDir)
	}
	for _, command := range []*string{&c.Planner.Executable, &c.Reviewer.Executable, &c.Implementer.Executable, &c.Git.Executable, &c.Fallback.CodexImplementer.Executable, &c.Fallback.OpenCodePlanner.Executable, &c.Fallback.OpenCodeReviewer.Executable} {
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
