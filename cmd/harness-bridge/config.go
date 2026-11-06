package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Config is the user's choice from setup, shared by every project: which agent
// plans, which implements, and whether to route requests first.
type Config struct {
	Planner     string   `json:"planner,omitempty"`
	Implementer string   `json:"implementer,omitempty"`
	Router      string   `json:"router,omitempty"`
	Model       string   `json:"model,omitempty"`
	Allow       []string `json:"allow,omitempty"`
}

func configPath() (string, error) {
	if path := os.Getenv("HARNESS_BRIDGE_CONFIG"); path != "" {
		return filepath.Abs(path)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("no user config directory; set HARNESS_BRIDGE_CONFIG: %w", err)
	}
	return filepath.Join(dir, "harness-bridge", "config.json"), nil
}

// loadConfig returns the saved config, or an empty one before setup has run.
func loadConfig() (Config, error) {
	var cfg Config
	path, err := configPath()
	if err != nil {
		return cfg, err
	}
	if err := readJSON(path, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	return cfg, nil
}

func saveConfig(cfg Config) (string, error) {
	path, err := configPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	return path, writeJSON(path, cfg)
}
