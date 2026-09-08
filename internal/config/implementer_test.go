package config

import "testing"

func TestImplementerSelectionPreservesExplicitSettings(t *testing.T) {
	file := configFile(t, `{"version":1,"implementer":{"harness":"codex","model":"file-luna","reasoning":"high"}}`)
	cfg, err := Load(file, t.TempDir(), environment(map[string]string{
		"MULTIHARNESS_IMPLEMENTER_MODEL": "env-luna",
	}), map[string]string{"implementer-model": "flag-luna"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Implementer.Harness != "codex" || cfg.Implementer.Executable != "codex" || cfg.Implementer.Model != "flag-luna" || cfg.Implementer.Reasoning != "high" || cfg.Implementer.Sandbox != "workspace-write" {
		t.Fatalf("Codex settings or precedence lost: %+v", cfg.Implementer)
	}
	// Old version-1 files keep their original OpenCode implementation semantics.
	old := configFile(t, `{"version":1,"implementer":{"model":"provider/old"}}`)
	cfg, err = Load(old, t.TempDir(), nil, nil)
	if err != nil || cfg.Implementer.Harness != "opencode" || cfg.Implementer.Model != "provider/old" {
		t.Fatalf("old configuration changed: %+v %v", cfg.Implementer, err)
	}
	for _, invalid := range []map[string]string{
		{"implementer-harness": "unknown"},
		{"implementer-harness": "codex", "implementer-model": ""},
		{"implementer-harness": "codex", "implementer-sandbox": "danger-full-access"},
		{"implementer-harness": "codex", "implementer-extra-args": `["--yolo"]`},
	} {
		if _, err := Load("", t.TempDir(), nil, invalid); err == nil {
			t.Fatal("invalid or unsafe implementation settings accepted")
		}
	}
}
