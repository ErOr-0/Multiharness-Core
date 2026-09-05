package sessionexec

import (
	"fmt"
	"strings"
	"time"
)

// ConfigurationError identifies one invalid OpenCode adapter setting.
type ConfigurationError struct {
	Field   string
	Message string
}

func (err *ConfigurationError) Error() string {
	return fmt.Sprintf("invalid OpenCode configuration %s: %s", err.Field, err.Message)
}

const (
	DefaultExecutable = "opencode"
	DefaultTimeout    = 60 * time.Minute
)

// PermissionPolicy controls how non-interactive OpenCode runs handle a
// permission that its configuration would otherwise ask the user to approve.
type PermissionPolicy string

const (
	// PermissionRejectOnPrompt keeps OpenCode's safe non-interactive behavior:
	// permission prompts are rejected instead of waiting for terminal input.
	PermissionRejectOnPrompt PermissionPolicy = "reject_on_prompt"
	// PermissionAutoApprove passes --auto. Explicit deny rules in OpenCode's
	// configuration still take precedence.
	PermissionAutoApprove PermissionPolicy = "auto_approve"
)

// Config contains immutable settings for one OpenCode role. Model uses
// OpenCode's provider/model form. An empty model lets OpenCode use its configured
// default; Variant may still select a variant of that default model.
type Config struct {
	Executable       string
	Model            string
	Variant          string
	Timeout          time.Duration
	PermissionPolicy PermissionPolicy
	ExtraArgs        []string
}

// DefaultConfig returns a safe, non-interactive configuration. It deliberately
// leaves Model and Variant empty because OpenCode provider availability is a
// deployment concern.
func DefaultConfig() Config {
	return Config{
		Executable:       DefaultExecutable,
		Timeout:          DefaultTimeout,
		PermissionPolicy: PermissionRejectOnPrompt,
	}
}

func (config Config) withDefaults() Config {
	defaults := DefaultConfig()
	if strings.TrimSpace(config.Executable) == "" {
		config.Executable = defaults.Executable
	} else {
		config.Executable = strings.TrimSpace(config.Executable)
	}
	config.Model = strings.TrimSpace(config.Model)
	config.Variant = strings.TrimSpace(config.Variant)
	if config.Timeout == 0 {
		config.Timeout = defaults.Timeout
	}
	if config.PermissionPolicy == "" {
		config.PermissionPolicy = defaults.PermissionPolicy
	}
	config.ExtraArgs = append([]string(nil), config.ExtraArgs...)
	return config
}

// Validate checks settings without accessing the filesystem or OpenCode.
func (config Config) Validate() error {
	if strings.TrimSpace(config.Executable) == "" {
		return &ConfigurationError{Field: "executable", Message: "must not be blank"}
	}
	if err := validateModel(config.Model); err != nil {
		return &ConfigurationError{Field: "model", Message: err.Error()}
	}
	if err := validateOptionalToken(config.Variant); err != nil {
		return &ConfigurationError{Field: "variant", Message: err.Error()}
	}
	if config.Timeout <= 0 {
		return &ConfigurationError{Field: "timeout", Message: "must be greater than zero"}
	}
	if !config.PermissionPolicy.valid() {
		return &ConfigurationError{
			Field:   "permission_policy",
			Message: fmt.Sprintf("unsupported value %q", config.PermissionPolicy),
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

func validateModel(model string) error {
	if model == "" {
		return nil
	}
	if err := validateOptionalToken(model); err != nil {
		return err
	}
	provider, modelID, found := strings.Cut(model, "/")
	if !found || provider == "" || modelID == "" || strings.HasSuffix(modelID, "/") {
		return fmt.Errorf("must use provider/model form")
	}
	return nil
}

func validateOptionalToken(value string) error {
	if strings.ContainsAny(value, " \t\r\n\x00") {
		return fmt.Errorf("must not contain whitespace or NUL")
	}
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("must not begin with a flag prefix")
	}
	return nil
}

func (policy PermissionPolicy) valid() bool {
	switch policy {
	case PermissionRejectOnPrompt, PermissionAutoApprove:
		return true
	default:
		return false
	}
}

var managedArguments = map[string]struct{}{
	"-c":                             {},
	"--continue":                     {},
	"-s":                             {},
	"--session":                      {},
	"-m":                             {},
	"--model":                        {},
	"--variant":                      {},
	"--format":                       {},
	"--dir":                          {},
	"--auto":                         {},
	"--yolo":                         {},
	"--dangerously-skip-permissions": {},
	"-i":                             {},
	"--interactive":                  {},
	"--mini":                         {},
	"--prompt":                       {},
	"--command":                      {},
	"--fork":                         {},
	"--attach":                       {},
	"--share":                        {},
	"-f":                             {},
	"--file":                         {},
	"--title":                        {},
	"--agent":                        {},
	"--thinking":                     {},
	"--replay-limit":                 {},
	"--no-replay":                    {},
	"--port":                         {},
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
	for _, shortName := range []string{"-c", "-s", "-m", "-i", "-f"} {
		if strings.HasPrefix(argument, shortName) {
			return fmt.Errorf("%q is managed by the adapter", shortName)
		}
	}
	return nil
}
