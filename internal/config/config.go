// Package config stores the user's setup choices, shared by every project.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"multiharness-core/internal/jsonfile"
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

func Path() (string, error) {
	if path := os.Getenv("HARNESS_BRIDGE_CONFIG"); path != "" {
		return filepath.Abs(path)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("no user config directory; set HARNESS_BRIDGE_CONFIG: %w", err)
	}
	return filepath.Join(dir, "harness-bridge", "config.json"), nil
}

// Load returns the saved config, or an empty one before setup has run.
func Load() (Config, error) {
	var cfg Config
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	if err := jsonfile.Read(path, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	return cfg, nil
}

func Save(cfg Config) (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	return path, jsonfile.Write(path, cfg)
}
