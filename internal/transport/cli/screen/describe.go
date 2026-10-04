package screen

import "multiharness-core/internal/config"

// HarnessName is the product name shown to people for a configured harness.
func HarnessName(harness string) string {
	switch harness {
	case "muse":
		return "Muse Code"
	case "claude":
		return "Claude"
	default:
		return "Codex"
	}
}

// PermissionDescription summarises the implementer's current access.
func PermissionDescription(cfg config.Config) string {
	choice := cfg.CurrentPermission()
	return choice.Label + ": " + choice.Detail
}
