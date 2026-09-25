package config

// PermissionChoice describes an application setting for the selected provider.
// CLI arguments belong to the provider adapters, not the terminal transport.
type PermissionChoice struct {
	Name, Label, Detail, Option, Value string
}

func (c Config) PermissionChoices() []PermissionChoice {
	switch c.Implementer.Harness {
	case "codex":
		choices := []PermissionChoice{{"workspace", "Workspace write", "Allow writes in the project sandbox; approval requests are rejected.", "implementer-sandbox", "workspace-write"}}
		if c.Mode == "direct" {
			choices = append(choices,
				PermissionChoice{"read-only", "Read only", "Keep Codex in its read-only sandbox; approval requests are rejected.", "implementer-sandbox", "read-only"},
				PermissionChoice{"full", "Full access", "Disable the Codex sandbox; commands can access files and network outside the project without approval.", "implementer-sandbox", "danger-full-access"})
		}
		return choices
	case "muse":
		return []PermissionChoice{{"native", "Workspace file edits", "File tools can edit the workspace. Shell execution is disabled; configured validation runs separately.", "implementer-permission-policy", "reject_on_prompt"}}
	case "claude":
		choices := []PermissionChoice{{"native", "Pre-approved tools only", "Use dontAsk: file tools are pre-approved; requests needing further approval are rejected.", "implementer-permission-policy", "reject_on_prompt"}}
		if c.Mode == "direct" {
			choices = append(choices,
				PermissionChoice{"edits", "Accept edits", "Use acceptEdits for file operations; other requests may still be rejected.", "implementer-permission-policy", "accept_edits"},
				PermissionChoice{"auto", "Automatic review", "Use Claude's auto mode to review requests; requires a supported model and account policy.", "implementer-permission-policy", "auto_approve"},
				PermissionChoice{"full", "Bypass permission prompts", "Use bypassPermissions for unattended execution; native deny rules and managed restrictions still apply.", "implementer-permission-policy", "bypass_permissions"})
		}
		return choices
	case "opencode":
		return []PermissionChoice{
			{"native", "Native rules", "Permission requests are rejected in non-interactive runs.", "implementer-permission-policy", "reject_on_prompt"},
			{"auto", "Auto-approve requests (--auto)", "Applies to all permission requests, including paths outside the project. Explicit deny rules in OpenCode still apply.", "implementer-permission-policy", "auto_approve"},
		}
	}
	return nil
}

func (c Config) CurrentPermission() PermissionChoice {
	for _, choice := range c.PermissionChoices() {
		value := string(c.Implementer.PermissionPolicy)
		if choice.Option == "implementer-sandbox" {
			value = string(c.Implementer.Sandbox)
		}
		if choice.Value == value {
			return choice
		}
	}
	return PermissionChoice{Label: "Unsupported permission setting"}
}
