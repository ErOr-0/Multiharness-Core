package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallationConfiguration(t *testing.T) {
	defaults := Defaults()
	if defaults.InstallMode != "prompt" || time.Duration(defaults.InstallTimeout) != 5*time.Minute {
		t.Fatal("unexpected installation defaults")
	}
	for _, tc := range []struct{ mode, timeout string }{
		{"always", "5m"}, {"yes", "5m"}, {"", "5m"}, {"prompt", "0s"}, {"prompt", "-1s"}, {"prompt", "31m"},
	} {
		if _, err := Load("", t.TempDir(), nil, map[string]string{"install-mode": tc.mode, "install-timeout": tc.timeout}); err == nil {
			t.Fatalf("accepted invalid setup %v", tc)
		}
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("{\"version\":1,\"install_mode\":\"disabled\",\"install_timeout\":\"2m\"}"), 0600); err != nil {
		t.Fatal(err)
	}
	lookup := func(key string) (string, bool) {
		if key == "MULTIHARNESS_INSTALL_MODE" {
			return "prompt", true
		}
		return "", false
	}
	cfg, err := Load(path, dir, lookup, map[string]string{"install-mode": "disabled", "install-timeout": "1m"})
	if err != nil || cfg.InstallMode != "disabled" || time.Duration(cfg.InstallTimeout) != time.Minute {
		t.Fatal("precedence failed", err)
	}
}
