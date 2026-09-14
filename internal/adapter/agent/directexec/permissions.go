package directexec

import "fmt"

func normalizePermissions(c *Config) error {
	if c.PermissionPolicy == "" {
		c.PermissionPolicy = "reject_on_prompt"
	}
	switch c.Harness {
	case "codex":
		if c.PermissionPolicy != "reject_on_prompt" {
			return fmt.Errorf("Codex permissions use the sandbox setting")
		}
		if c.Sandbox == "" {
			c.Sandbox = "workspace-write"
		}
		switch c.Sandbox {
		case "read-only", "workspace-write", "danger-full-access":
			return nil
		}
	case "claude":
		if claudePermissionMode(c.PermissionPolicy) != "" {
			return nil
		}
	case "opencode":
		if c.PermissionPolicy == "reject_on_prompt" || c.PermissionPolicy == "auto_approve" {
			return nil
		}
	}
	return fmt.Errorf("unsupported %s permission setting", c.Harness)
}

func claudePermissionMode(policy string) string {
	switch policy {
	case "reject_on_prompt":
		return "dontAsk"
	case "accept_edits":
		return "acceptEdits"
	case "auto_approve":
		return "auto"
	case "bypass_permissions":
		return "bypassPermissions"
	}
	return ""
}
