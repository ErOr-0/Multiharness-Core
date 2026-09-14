package config

import (
	"testing"
	"time"
)

func TestModeDefaultsAndLegacyConfiguration(t *testing.T) {
	legacy := configFile(t, `{"version":1,"implementer":{"harness":"opencode","model":"provider/saved","timeout":"45m"}}`)
	cfg, err := Load(legacy, t.TempDir(), nil, nil)
	if err != nil || cfg.Mode != "direct" || cfg.Implementer.Model != "provider/saved" {
		t.Fatalf("legacy selection lost: %+v %v", cfg, err)
	}
	deadline, name := cfg.DirectTimeout()
	if deadline != 45*time.Minute || name != "implementer-timeout" {
		t.Fatalf("%s %s", deadline, name)
	}
	cfg, err = Load(legacy, t.TempDir(), environment(map[string]string{"MULTIHARNESS_MODE": "team"}), map[string]string{"mode": "direct", "timeout": "10m"})
	if err != nil || cfg.Mode != "direct" {
		t.Fatalf("mode precedence: %+v %v", cfg, err)
	}
	deadline, name = cfg.DirectTimeout()
	if deadline != 10*time.Minute || name != "timeout" {
		t.Fatalf("%s %s", deadline, name)
	}
	cfg, err = Load(legacy, t.TempDir(), nil, map[string]string{"mode": "team"})
	if err != nil || cfg.Mode != "team" {
		t.Fatal(cfg, err)
	}
	if _, err := Load("", t.TempDir(), nil, map[string]string{"mode": "automatic"}); err == nil {
		t.Fatal("accepted unknown mode")
	}
}
