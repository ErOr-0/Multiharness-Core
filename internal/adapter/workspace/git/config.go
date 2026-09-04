package git

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Config bounds checkout inspection. Unsupported/oversized inputs fail closed;
// no partial snapshot or truncated diff can be approved.
type Config struct {
	Executable       string
	Timeout          time.Duration
	MaxFiles         int
	MaxFileBytes     int64
	MaxSnapshotBytes int64
	MaxOutputBytes   int
}

func DefaultConfig() Config {
	return Config{Executable: "git", Timeout: 30 * time.Second, MaxFiles: 20000,
		MaxFileBytes: 8 << 20, MaxSnapshotBytes: 64 << 20, MaxOutputBytes: 4 << 20}
}

func (config Config) defaults() (Config, error) {
	d := DefaultConfig()
	if config.Executable == "" {
		config.Executable = d.Executable
	}
	if config.Timeout == 0 {
		config.Timeout = d.Timeout
	}
	if config.MaxFiles == 0 {
		config.MaxFiles = d.MaxFiles
	}
	if config.MaxFileBytes == 0 {
		config.MaxFileBytes = d.MaxFileBytes
	}
	if config.MaxSnapshotBytes == 0 {
		config.MaxSnapshotBytes = d.MaxSnapshotBytes
	}
	if config.MaxOutputBytes == 0 {
		config.MaxOutputBytes = d.MaxOutputBytes
	}
	if config.Timeout < 0 || config.MaxFiles < 1 || config.MaxFileBytes < 1 || config.MaxSnapshotBytes < 1 || config.MaxOutputBytes < 1 {
		return Config{}, fmt.Errorf("Git inspection timeout and limits must be positive")
	}
	if strings.TrimSpace(config.Executable) == "" || strings.ContainsRune(config.Executable, 0) {
		return Config{}, fmt.Errorf("Git executable is invalid")
	}
	if config.MaxFileBytes == math.MaxInt64 {
		return Config{}, fmt.Errorf("file byte limit is too large")
	}
	return config, nil
}
