package schemaexec

import (
	"fmt"
	"strings"
	"time"
)

const (
	DefaultExecutable = "codex"
	DefaultModel      = "gpt-5.6-sol"
	DefaultReasoning  = "xhigh"
	DefaultTimeout    = 30 * time.Minute
)

// SandboxMode is a Codex CLI sandbox policy.
type SandboxMode string

const (
	SandboxReadOnly         SandboxMode = "read-only"
	SandboxWorkspaceWrite   SandboxMode = "workspace-write"
	SandboxDangerFullAccess SandboxMode = "danger-full-access"
)

// Config contains immutable settings shared by one Codex adapter instance.
type Config struct {
	Executable string
	Model      string
	Reasoning  string
	Timeout    time.Duration
	Sandbox    SandboxMode
	ExtraArgs  []string
}

// DefaultConfig returns the recommended planning and review configuration.
func DefaultConfig() Config {
	return Config{
		Executable: DefaultExecutable,
		Model:      DefaultModel,
		Reasoning:  DefaultReasoning,
		Timeout:    DefaultTimeout,
		Sandbox:    SandboxReadOnly,
	}
}

func (config Config) withDefaults() Config {
	defaults := DefaultConfig()
	if strings.TrimSpace(config.Executable) == "" {
		config.Executable = defaults.Executable
	} else {
		config.Executable = strings.TrimSpace(config.Executable)
	}
	if strings.TrimSpace(config.Model) == "" {
		config.Model = defaults.Model
	} else {
		config.Model = strings.TrimSpace(config.Model)
	}
	if strings.TrimSpace(config.Reasoning) == "" {
		config.Reasoning = defaults.Reasoning
	} else {
		config.Reasoning = strings.TrimSpace(config.Reasoning)
	}
	if config.Timeout == 0 {
		config.Timeout = defaults.Timeout
	}
	if config.Sandbox == "" {
		config.Sandbox = defaults.Sandbox
	}
	config.ExtraArgs = append([]string(nil), config.ExtraArgs...)
	return config
}

// Validate checks settings without accessing the filesystem or Codex.
func (config Config) Validate() error {
	if strings.TrimSpace(config.Executable) == "" {
		return &ConfigurationError{Field: "executable", Message: "must not be blank"}
	}
	if strings.TrimSpace(config.Model) == "" {
		return &ConfigurationError{Field: "model", Message: "must not be blank"}
	}
	if !validReasoning(config.Reasoning) {
		return &ConfigurationError{
			Field:   "reasoning",
			Message: fmt.Sprintf("unsupported value %q", config.Reasoning),
		}
	}
	if config.Timeout <= 0 {
		return &ConfigurationError{Field: "timeout", Message: "must be greater than zero"}
	}
	if !config.Sandbox.valid() {
		return &ConfigurationError{
			Field:   "sandbox",
			Message: fmt.Sprintf("unsupported value %q", config.Sandbox),
		}
	}
	for index, argument := range config.ExtraArgs {
		if err := validateExtraArgument(argument); err != nil {
			return &ConfigurationError{
				Field:   fmt.Sprintf("extra_args[%d]", index),
				Message: err.Error(),
			}
		}
	}
	return nil
}

func validReasoning(reasoning string) bool {
	switch reasoning {
	case "none", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func (mode SandboxMode) valid() bool {
	switch mode {
	case SandboxReadOnly, SandboxWorkspaceWrite, SandboxDangerFullAccess:
		return true
	default:
		return false
	}
}

var managedArguments = map[string]struct{}{
	"-p":                    {},
	"-o":                    {},
	"-C":                    {},
	"--cd":                  {},
	"-c":                    {},
	"--config":              {},
	"-m":                    {},
	"--model":               {},
	"-s":                    {},
	"--sandbox":             {},
	"--add-dir":             {},
	"--color":               {},
	"--ephemeral":           {},
	"--full-auto":           {},
	"--json":                {},
	"--output-last-message": {},
	"--output-schema":       {},
	"--image":               {},
	"--profile":             {},
	"--oss":                 {},
	"--local-provider":      {},
	"--ignore-rules":        {},
	"--dangerously-bypass-approvals-and-sandbox": {},
	"--dangerously-bypass-hook-trust":            {},
}

func validateExtraArgument(argument string) error {
	if strings.TrimSpace(argument) == "" {
		return fmt.Errorf("must not be blank")
	}
	if strings.ContainsAny(argument, " \t\r\n\x00") {
		return fmt.Errorf("must be one flag token without whitespace or NUL")
	}
	if argument == "-" || argument == "--" || !strings.HasPrefix(argument, "-") {
		return fmt.Errorf("must be a flag; use --flag=value when a value is required")
	}
	name, _, _ := strings.Cut(argument, "=")
	if _, managed := managedArguments[name]; managed {
		return fmt.Errorf("%q is managed by the adapter", name)
	}
	for _, short := range []string{"-C", "-c", "-m", "-s", "-p", "-o"} {
		if strings.HasPrefix(argument, short) {
			return fmt.Errorf("%q is managed by the adapter", short)
		}
	}
	return nil
}

// ConfigurationError identifies one invalid Codex adapter setting.
type ConfigurationError struct {
	Field   string
	Message string
}

func (err *ConfigurationError) Error() string {
	return fmt.Sprintf("invalid Codex configuration %s: %s", err.Field, err.Message)
}
