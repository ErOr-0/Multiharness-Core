package folder

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Config controls folder inspection. Zero size/count limits mean unlimited.
// Explicit positive limits fail closed; partial evidence cannot be approved.
type Config struct {
	ExistingWork     string
	RecoveryDir      string
	Timeout          time.Duration
	MaxFiles         int
	MaxFileBytes     int64
	MaxSnapshotBytes int64
	MaxOutputBytes   int
}

func DefaultConfig() Config {
	return Config{ExistingWork: "snapshot", Timeout: 30 * time.Second}
}

func (config Config) defaults() (Config, error) {
	d := DefaultConfig()
	if config.Timeout == 0 {
		config.Timeout = d.Timeout
	}
	if config.Timeout < 0 || config.MaxFiles < 0 || config.MaxFileBytes < 0 || config.MaxSnapshotBytes < 0 || config.MaxOutputBytes < 0 {
		return Config{}, fmt.Errorf("Folder inspection timeout must be positive and limits must be nonnegative")
	}
	if config.MaxFileBytes == math.MaxInt64 {
		return Config{}, fmt.Errorf("file byte limit is too large")
	}
	if config.ExistingWork == "" {
		config.ExistingWork = d.ExistingWork
	}
	switch config.ExistingWork {
	case "preserve", "prompt", "snapshot":
	default:
		return Config{}, fmt.Errorf("existing work must be prompt, preserve or snapshot")
	}
	if strings.ContainsRune(config.RecoveryDir, 0) {
		return Config{}, fmt.Errorf("recovery directory contains NUL")
	}
	return config, nil
}
