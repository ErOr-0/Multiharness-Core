package config

import (
	"fmt"
	"math"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/schemaexec"
	"multiharness-core/internal/adapter/agent/sessionexec"
	"multiharness-core/internal/adapter/process"
	validationadapter "multiharness-core/internal/adapter/validation"
	gitworkspace "multiharness-core/internal/adapter/workspace/git"
)

// Validate checks the winning configuration without invoking commands or models.
// Each section owns its validation; precedence and path resolution live in Load.
func (c Config) Validate() error {
	if err := c.validateInstallation(); err != nil {
		return err
	}
	if err := c.Fallback.validate(); err != nil {
		return err
	}
	if err := c.validateRun(); err != nil {
		return err
	}
	if err := c.Execution.validate(); err != nil {
		return err
	}
	if err := c.validateAgents(); err != nil {
		return err
	}
	if err := c.Git.validate(); err != nil {
		return err
	}
	return c.Validation.validate()
}

func (c Config) validateInstallation() error {
	if c.InstallMode != "prompt" && c.InstallMode != "disabled" {
		return fmt.Errorf("install_mode must be prompt or disabled")
	}
	if c.InstallTimeout <= 0 || time.Duration(c.InstallTimeout) > 30*time.Minute {
		return fmt.Errorf("install_timeout must be positive and at most 30m")
	}
	return nil
}

func (f Fallback) validate() error {
	if f.Mode != "prompt" && f.Mode != "disabled" {
		return fmt.Errorf("fallback.mode must be prompt or disabled; unattended switching is not permitted")
	}
	if err := f.CodexImplementer.Adapter().Validate(); err != nil {
		return fmt.Errorf("fallback.codex_implementer: %w", err)
	}
	if err := executable(f.CodexImplementer.Executable); err != nil {
		return err
	}
	if f.CodexImplementer.Sandbox != schemaexec.SandboxWorkspaceWrite {
		return fmt.Errorf("fallback.codex_implementer.sandbox must be workspace-write")
	}
	model := f.CodexImplementer.Model
	if strings.TrimSpace(model) == "" || strings.ContainsAny(model, " \t\r\n\x00") || strings.HasPrefix(model, "-") {
		return fmt.Errorf("fallback.codex_implementer.model must be a model identifier")
	}
	if err := f.OpenCodeReviewer.Adapter().Validate(); err != nil {
		return fmt.Errorf("fallback.opencode_reviewer: %w", err)
	}
	if err := executable(f.OpenCodeReviewer.Executable); err != nil {
		return err
	}
	if f.OpenCodeReviewer.PermissionPolicy != sessionexec.PermissionRejectOnPrompt {
		return fmt.Errorf("fallback review requires reject_on_prompt")
	}
	if err := f.Planner.validate(); err != nil {
		return fmt.Errorf("fallback.planner: %w", err)
	}
	return nil
}

func (c Config) validateRun() error {
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
	if strings.TrimSpace(c.WorkingDir) == "" || strings.ContainsRune(c.WorkingDir, 0) {
		return fmt.Errorf("working_dir must be a nonempty path without NUL")
	}
	if c.MaxRepairAttempts < 0 {
		return fmt.Errorf("max_repair_attempts must be zero or greater")
	}
	if c.Timeout <= 0 || c.MaxTaskBytes <= 0 || int64(c.MaxTaskBytes) == math.MaxInt64 {
		return fmt.Errorf("timeout and max_task_bytes must be positive")
	}
	return nil
}

func (e Execution) validate() error {
	if err := e.Policy().Validate(); err != nil {
		return fmt.Errorf("execution: %w", err)
	}
	if e.MaxCostMicrousd != 0 {
		return fmt.Errorf("execution.max_cost_microusd: monetary caps cannot be enforced by the CLI adapters; use enforced provider billing limits or a metered gateway (0 disables this unsupported cap)")
	}
	return nil
}

func (c Config) validateAgents() error {
	if err := c.Planner.validate(); err != nil {
		return fmt.Errorf("planner: %w", err)
	}
	if c.Fallback.Mode != "disabled" && c.Fallback.Planner.Harness == c.Planner.Harness {
		return fmt.Errorf("fallback.planner.harness must differ from planner.harness")
	}
	if err := executable(c.Reviewer.Executable); err != nil {
		return fmt.Errorf("reviewer.executable: %w", err)
	}
	if strings.TrimSpace(c.Reviewer.Model) == "" || strings.ContainsAny(c.Reviewer.Model, " \t\r\n\x00") || strings.HasPrefix(c.Reviewer.Model, "-") {
		return fmt.Errorf("reviewer.model must be a nonempty model identifier")
	}
	if err := c.Reviewer.Adapter().Validate(); err != nil {
		return fmt.Errorf("reviewer: %w", err)
	}
	if c.Reviewer.Sandbox != schemaexec.SandboxReadOnly {
		return fmt.Errorf("reviewer.sandbox must be read-only")
	}
	if err := executable(c.Implementer.Executable); err != nil {
		return fmt.Errorf("implementer.executable: %w", err)
	}
	if err := c.Implementer.validate(); err != nil {
		return fmt.Errorf("implementer: %w", err)
	}
	return nil
}

func (g Git) validate() error {
	if err := executable(g.Executable); err != nil {
		return fmt.Errorf("git.executable: %w", err)
	}
	if g.Timeout <= 0 || g.MaxFiles < 0 || g.MaxFileBytes < 0 || g.MaxSnapshotBytes < 0 || g.MaxOutputBytes < 0 {
		return fmt.Errorf("Git timeout must be positive and limits must be nonnegative")
	}
	if _, err := gitworkspace.NewWorkspace(process.NewOSRunner(), g.Adapter()); err != nil {
		return err
	}
	return nil
}

func (v Validation) validate() error {
	if v.DefaultTimeout <= 0 || v.OutputLimit <= 0 {
		return fmt.Errorf("validation timeout and output_limit must be positive")
	}
	for i, check := range v.Checks {
		if err := executable(check.Executable); err != nil {
			return fmt.Errorf("validation.checks[%d].executable: %w", i, err)
		}
	}
	if _, err := validationadapter.NewValidator(process.NewOSRunner(), v.Adapter()); err != nil {
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
