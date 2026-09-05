package config

import (
	"time"

	"multiharness-core/internal/adapter/agent/schemaexec"
	"multiharness-core/internal/adapter/agent/sessionexec"
	validationadapter "multiharness-core/internal/adapter/validation"
	gitworkspace "multiharness-core/internal/adapter/workspace/git"
)

func (c Codex) Adapter() schemaexec.Config {
	return schemaexec.Config{
		Executable: c.Executable,
		Model:      c.Model,
		Reasoning:  c.Reasoning,
		Timeout:    time.Duration(c.Timeout),
		Sandbox:    c.Sandbox,
		ExtraArgs:  c.ExtraArgs,
	}
}
func (c OpenCode) Adapter() sessionexec.Config {
	return sessionexec.Config{
		Executable:       c.Executable,
		Model:            c.Model,
		Variant:          c.Variant,
		Timeout:          time.Duration(c.Timeout),
		PermissionPolicy: c.PermissionPolicy,
		ExtraArgs:        c.ExtraArgs,
	}
}
func (c Git) Adapter() gitworkspace.Config {
	return gitworkspace.Config{
		Executable:       c.Executable,
		Timeout:          time.Duration(c.Timeout),
		MaxFiles:         c.MaxFiles,
		MaxFileBytes:     c.MaxFileBytes,
		MaxSnapshotBytes: c.MaxSnapshotBytes,
		MaxOutputBytes:   c.MaxOutputBytes,
	}
}
func (c Validation) Adapter() validationadapter.Config {
	checks := make([]validationadapter.Check, 0, len(c.Checks))
	for _, check := range c.Checks {
		checks = append(
			checks,
			validationadapter.Check{
				Executable:   check.Executable,
				Args:         check.Args,
				Timeout:      time.Duration(check.Timeout),
				EnvOverrides: check.EnvOverrides,
			},
		)
	}
	return validationadapter.Config{Checks: checks, DefaultTimeout: time.Duration(c.DefaultTimeout), OutputLimit: c.OutputLimit}
}
