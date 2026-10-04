package config

import (
	"fmt"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/schemaexec"
)

// Planner configures one role. The composition root selects the provider adapter.
// Provider-specific settings remain explicit; no second primary planner is stored.
type Planner struct {
	Harness          string                      `json:"harness"`
	Executable       string                      `json:"executable"`
	Model            string                      `json:"model"`
	Reasoning        string                      `json:"reasoning"`
	Timeout          Duration                    `json:"timeout"`
	Sandbox          schemaexec.SandboxMode      `json:"sandbox"`
	PermissionPolicy schemaexec.PermissionPolicy `json:"permission_policy"`
	ExtraArgs        []string                    `json:"extra_args"`
}

func DefaultPlanner(harness string) Planner {
	codex := schemaexec.DefaultConfig()
	p := Planner{
		Harness: harness, Executable: codex.Executable, Model: codex.Model,
		Reasoning: codex.Reasoning, Timeout: Duration(codex.Timeout),
		Sandbox: schemaexec.SandboxReadOnly, PermissionPolicy: schemaexec.PermissionRejectOnPrompt,
		ExtraArgs: []string{},
	}
	if harness == "muse" {
		p.Executable, p.Model, p.Reasoning = "muse", "muse-spark-1.3", "high"
	}
	if harness == "claude" {
		p.Executable, p.Model, p.Reasoning = "claude", "sonnet", "high"
	}
	return p
}

func (p Planner) CodexAdapter() schemaexec.Config {
	return (Codex{p.Executable, p.Model, p.Reasoning, p.Timeout, p.Sandbox, p.ExtraArgs}).Adapter()
}

func (p Planner) validate() error {
	if err := executable(p.Executable); err != nil {
		return err
	}
	if p.Sandbox != schemaexec.SandboxReadOnly || p.PermissionPolicy != schemaexec.PermissionRejectOnPrompt {
		return fmt.Errorf("read-only role requires read-only sandbox and reject_on_prompt permissions")
	}
	switch p.Harness {
	case "codex":
		if strings.TrimSpace(p.Model) == "" || strings.ContainsAny(p.Model, " \t\r\n\x00") || strings.HasPrefix(p.Model, "-") {
			return fmt.Errorf("model must be a nonempty model identifier")
		}
		return p.CodexAdapter().Validate()
	case "muse":
		return p.MuseAdapter().Validate()
	case "claude":
		return p.ClaudeAdapter().Validate()
	default:
		return fmt.Errorf("harness must be codex, claude or muse")
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
	} {
		if !supplied[prefix+field.name] {
			*field.target = field.value
		}
	}
}

func (p Planner) ClaudeAdapter() schemaexec.ClaudeConfig {
	return schemaexec.ClaudeConfig{Executable: p.Executable, Model: p.Model, Effort: p.Reasoning, Timeout: time.Duration(p.Timeout), ExtraArgs: p.ExtraArgs}
}

func (p Planner) MuseAdapter() schemaexec.MuseConfig {
	return schemaexec.MuseConfig{Executable: p.Executable, Model: p.Model, Reasoning: p.Reasoning, Timeout: time.Duration(p.Timeout), ExtraArgs: p.ExtraArgs}
}
