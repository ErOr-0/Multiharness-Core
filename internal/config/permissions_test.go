package config

import "testing"

func TestAdvertisedPermissionChoicesValidateForTheirProviderAndMode(t *testing.T) {
	for _, mode := range []string{"direct", "team"} {
		for _, harness := range []string{"codex", "claude", "opencode"} {
			cfg, err := Load("", t.TempDir(), nil, map[string]string{"mode": mode, "implementer-harness": harness})
			if err != nil {
				t.Fatal(err)
			}
			for _, choice := range cfg.PermissionChoices() {
				updated, err := Load("", t.TempDir(), nil, map[string]string{"mode": mode, "implementer-harness": harness, choice.Option: choice.Value})
				if err != nil || updated.CurrentPermission().Name != choice.Name {
					t.Fatal(mode, harness, choice, err)
				}
			}
		}
	}
	for _, harness := range []string{"codex", "claude", "opencode"} {
		if _, err := Load("", t.TempDir(), nil, map[string]string{"implementer-harness": harness, "implementer-permission-policy": "unknown"}); err == nil {
			t.Fatal("accepted unknown permission", harness)
		}
	}
}
