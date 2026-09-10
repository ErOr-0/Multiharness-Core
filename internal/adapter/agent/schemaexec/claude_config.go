package schemaexec

import (
	"errors"
	"strings"
	"time"
)

type ClaudeConfig struct {
	Executable, Model, Effort string
	Timeout                   time.Duration
	CanWrite                  bool
	ExtraArgs                 []string
}

func (c ClaudeConfig) Validate() error {
	if strings.TrimSpace(c.Executable) == "" || strings.TrimSpace(c.Model) == "" || strings.ContainsAny(c.Model, " \t\r\n\x00") || strings.HasPrefix(c.Model, "-") {
		return errors.New("Claude executable and model must be nonempty identifiers")
	}
	if c.Timeout <= 0 || c.Timeout > 24*time.Hour {
		return errors.New("Claude timeout must be positive and at most 24h")
	}
	switch c.Effort {
	case "", "low", "medium", "high", "xhigh", "max":
	default:
		return errors.New("Claude effort must be low, medium, high, xhigh or max")
	}
	// Additional flags can replace permissions, sessions, tools or structured output.
	// Keep this boundary closed until a concrete safe extension is required.
	if len(c.ExtraArgs) > 0 {
		return errors.New("Claude extra_args are not supported; use the explicit role settings")
	}
	return nil
}
