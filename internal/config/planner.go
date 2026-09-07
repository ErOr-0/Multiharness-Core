package config

import (
	"fmt"
	"strings"

	"multiharness-core/internal/adapter/agent/schemaexec"
	"multiharness-core/internal/adapter/agent/sessionexec"
)

// Planner configures one role. The composition root selects the provider adapter.
// Provider-specific settings remain explicit; no second primary planner is stored.
type Planner struct {
	Harness          string                       `json:"harness"`
	Executable       string                       `json:"executable"`
	Model            string                       `json:"model"`
	Reasoning        string                       `json:"reasoning"`
	Variant          string                       `json:"variant"`
	Timeout          Duration                     `json:"timeout"`
	Sandbox          schemaexec.SandboxMode       `json:"sandbox"`
	PermissionPolicy sessionexec.PermissionPolicy `json:"permission_policy"`
	ExtraArgs        []string                     `json:"extra_args"`
}

func DefaultPlanner(harness string) Planner {
	codex := schemaexec.DefaultConfig()
	p := Planner{
		Harness: harness, Executable: codex.Executable, Model: codex.Model,
		Reasoning: codex.Reasoning, Timeout: Duration(codex.Timeout),
		Sandbox: schemaexec.SandboxReadOnly, PermissionPolicy: sessionexec.PermissionRejectOnPrompt,
		ExtraArgs: []string{},
	}
	if harness == "opencode" {
		opencode := sessionexec.DefaultConfig()
		p.Executable, p.Model, p.Reasoning = opencode.Executable, opencode.Model, ""
		p.Variant = opencode.Variant
	}
	return p
}

func (p Planner) CodexAdapter() schemaexec.Config {
	return (Codex{p.Executable, p.Model, p.Reasoning, p.Timeout, p.Sandbox, p.ExtraArgs}).Adapter()
}

func (p Planner) OpenCodeAdapter() sessionexec.Config {
	return (OpenCode{p.Executable, p.Model, p.Variant, p.Timeout, p.PermissionPolicy, p.ExtraArgs}).Adapter()
}

func (p Planner) validate() error {
	if err := executable(p.Executable); err != nil {
		return err
	}
	if p.Sandbox != schemaexec.SandboxReadOnly || p.PermissionPolicy != sessionexec.PermissionRejectOnPrompt {
		return fmt.Errorf("planning requires read-only sandbox and reject_on_prompt permissions")
	}
	switch p.Harness {
	case "codex":
		if strings.TrimSpace(p.Model) == "" || strings.ContainsAny(p.Model, " \t\r\n\x00") || strings.HasPrefix(p.Model, "-") {
			return fmt.Errorf("model must be a nonempty model identifier")
		}
		return p.CodexAdapter().Validate()
	case "opencode":
		return p.OpenCodeAdapter().Validate()
	default:
		return fmt.Errorf("harness must be codex or opencode")
	}
}

// Only omitted values use provider defaults. Explicit models, executable pins,
// empty strings and higher-precedence settings must never be silently replaced.
func (p *Planner) resolveDefaults(prefix string, supplied map[string]bool) {
	defaults := DefaultPlanner(p.Harness)
	for _, field := range []struct {
		name   string
		target *string
		value  string
	}{
		{"executable", &p.Executable, defaults.Executable},
		{"model", &p.Model, defaults.Model},
		{"reasoning", &p.Reasoning, defaults.Reasoning},
		{"variant", &p.Variant, defaults.Variant},
	} {
		if !supplied[prefix+field.name] {
			*field.target = field.value
		}
	}
}
