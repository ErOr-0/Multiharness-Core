package config

// PermissionChoice describes an application setting for the selected provider.
// CLI arguments belong to the provider adapters, not the terminal transport.
type PermissionChoice struct {
	Name, Label, Detail, Option, Value string
}

func (c Config) PermissionChoices() []PermissionChoice {
	switch c.Implementer.Harness {
	case "codex":
		choices := []PermissionChoice{{"workspace", "Workspace write", "Allow project sandbox writes. Native approval requests appear here in an interactive terminal; unattended requests are rejected.", "implementer-sandbox", "workspace-write"}}
		if c.Mode == "direct" {
			choices = append(choices,
				PermissionChoice{"read-only", "Read only", "Keep Codex in its read-only sandbox; approval requests are rejected.", "implementer-sandbox", "read-only"},
				PermissionChoice{"full", "Full access", "Disable the Codex sandbox; commands can access files and network outside the project without approval.", "implementer-sandbox", "danger-full-access"})
		}
		return choices
	case "muse":
		return []PermissionChoice{
			{"native", "Workspace file edits", "Native file permission requests appear here. Shell execution is disabled; configured validation runs separately.", "implementer-permission-policy", "reject_on_prompt"},
			{"confirm", "Approved shell commands", "Muse may run commands such as tests; each command Muse does not already trust waits for your decision here. Workspace file edits never ask in Muse. Needs an interactive terminal; unattended runs stop before starting.", "implementer-permission-policy", "confirm"},
		}
	case "claude":
		choices := []PermissionChoice{{"native", "Native rules and approvals", "Show Claude permission requests here and send your decision back to Claude. Unattended requests are rejected.", "implementer-permission-policy", "reject_on_prompt"}}
		if c.Mode == "direct" {
			choices = append(choices,
				PermissionChoice{"edits", "Accept edits", "Use acceptEdits for file operations; other native approval requests appear here when interactive.", "implementer-permission-policy", "accept_edits"},
				PermissionChoice{"auto", "Automatic review", "Use Claude's auto mode to review requests; requires a supported model and account policy.", "implementer-permission-policy", "auto_approve"},
				PermissionChoice{"full", "Bypass permission prompts", "Use bypassPermissions for unattended execution; native deny rules and managed restrictions still apply.", "implementer-permission-policy", "bypass_permissions"})
		}
		return choices
	case "opencode":
		return []PermissionChoice{
			{"native", "Native rules", "Show OpenCode permission requests here; unattended requests are rejected.", "implementer-permission-policy", "reject_on_prompt"},
			{"auto", "Auto-approve requests (--auto)", "Applies to all permission requests, including paths outside the project. Explicit deny rules in OpenCode still apply.", "implementer-permission-policy", "auto_approve"},
			{"confirm", "Ask before every change", "Every file edit, command and web fetch waits for your decision here; reading the project does not. Needs an interactive terminal; unattended runs stop before starting.", "implementer-permission-policy", "confirm"},
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
