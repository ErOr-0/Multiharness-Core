package git

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Config controls checkout inspection. Zero size/count limits mean unlimited.
// Explicit positive limits fail closed; partial evidence cannot be approved.
type Config struct {
	Executable       string
	Timeout          time.Duration
	MaxFiles         int
	MaxFileBytes     int64
	MaxSnapshotBytes int64
	MaxOutputBytes   int
}

func DefaultConfig() Config {
	return Config{Executable: "git", Timeout: 30 * time.Second}
}

func (config Config) defaults() (Config, error) {
	d := DefaultConfig()
	if config.Executable == "" {
		config.Executable = d.Executable
	}
	if config.Timeout == 0 {
		config.Timeout = d.Timeout
	}
	if config.Timeout < 0 || config.MaxFiles < 0 || config.MaxFileBytes < 0 || config.MaxSnapshotBytes < 0 || config.MaxOutputBytes < 0 {
		return Config{}, fmt.Errorf("Git inspection timeout must be positive and limits must be nonnegative")
	}
	if strings.TrimSpace(config.Executable) == "" || strings.ContainsRune(config.Executable, 0) {
		return Config{}, fmt.Errorf("Git executable is invalid")
	}
	if config.MaxFileBytes == math.MaxInt64 {
		return Config{}, fmt.Errorf("file byte limit is too large")
	}
	return config, nil
}
