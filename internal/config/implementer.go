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

func (i Implementer) validate(mode string) error {
	if err := executable(i.Executable); err != nil {
		return err
	}
	if (mode != "direct" || i.Harness != "codex") && i.Sandbox != schemaexec.SandboxWorkspaceWrite {
		return fmt.Errorf("implementer.sandbox must be workspace-write")
	}
	switch i.Harness {
	case "codex":
		if strings.TrimSpace(i.Model) == "" || strings.ContainsAny(i.Model, " \t\r\n\x00") || strings.HasPrefix(i.Model, "-") {
			return fmt.Errorf("model must be a nonempty model identifier")
		}
		if i.PermissionPolicy != sessionexec.PermissionRejectOnPrompt {
			return fmt.Errorf("Codex uses implementer.sandbox; permission_policy must be reject_on_prompt")
		}
		return i.CodexAdapter().Validate()
	case "muse":
		if i.PermissionPolicy != sessionexec.PermissionRejectOnPrompt {
			return fmt.Errorf("Muse requires reject_on_prompt permissions")
		}
		return i.MuseAdapter().Validate()
	case "claude":
		if mode == "direct" {
			switch i.PermissionPolicy {
			case sessionexec.PermissionRejectOnPrompt, "accept_edits", "auto_approve", "bypass_permissions":
			default:
				return fmt.Errorf("unsupported Claude permission policy")
			}
		} else if i.PermissionPolicy != sessionexec.PermissionRejectOnPrompt {
			return fmt.Errorf("Claude implementation requires reject_on_prompt permissions")
		}
		return i.ClaudeAdapter().Validate()
	case "opencode":
		return i.OpenCodeAdapter().Validate()
	default:
		return fmt.Errorf("harness must be codex, opencode, claude or muse")
	}
}

func (i Implementer) ClaudeAdapter() schemaexec.ClaudeConfig {
	c := Planner(i).ClaudeAdapter()
	c.CanWrite = true
	return c
}

func (i Implementer) MuseAdapter() schemaexec.MuseConfig {
	c := Planner(i).MuseAdapter()
	c.CanWrite = true
	return c
}
