package config

import (
	"fmt"
	"strings"

	"multiharness-core/internal/adapter/agent/schemaexec"
	"multiharness-core/internal/adapter/agent/sessionexec"
)

// Implementer has the same provider settings as Planner, with a distinct write
// policy. The role remains provider-neutral until the composition root.
type Implementer Planner

func DefaultImplementer(harness string) Implementer {
	p := Implementer(DefaultPlanner(harness))
	p.Sandbox = schemaexec.SandboxWorkspaceWrite
	p.Timeout = Duration(sessionexec.DefaultConfig().Timeout)
	return p
}

func (i Implementer) CodexAdapter() schemaexec.Config {
	return Planner(i).CodexAdapter()
}

func (i Implementer) OpenCodeAdapter() sessionexec.Config {
	return Planner(i).OpenCodeAdapter()
}

func (i *Implementer) resolveDefaults(supplied map[string]bool) {
	// Only provider-specific identifiers change; explicit values and this role's
	// independent timeout and write policy remain intact.
	p := Planner(*i)
	p.resolveDefaults("implementer.", supplied)
	*i = Implementer(p)
}

func (i Implementer) validate() error {
	if err := executable(i.Executable); err != nil {
		return err
	}
	if i.Sandbox != schemaexec.SandboxWorkspaceWrite {
		return fmt.Errorf("implementer.sandbox must be workspace-write")
	}
	switch i.Harness {
	case "codex":
		if strings.TrimSpace(i.Model) == "" || strings.ContainsAny(i.Model, " \t\r\n\x00") || strings.HasPrefix(i.Model, "-") {
			return fmt.Errorf("model must be a nonempty model identifier")
		}
		if i.PermissionPolicy != sessionexec.PermissionRejectOnPrompt {
			return fmt.Errorf("Codex implementation uses workspace-write; OpenCode auto_approve does not apply")
		}
		return i.CodexAdapter().Validate()
	case "opencode":
		return i.OpenCodeAdapter().Validate()
	default:
		return fmt.Errorf("harness must be codex or opencode")
	}
}
