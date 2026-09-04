package opencode

import (
	"io"
	"strings"

	"multiharness-core/internal/adapter/process"
)

func buildCommand(
	config Config,
	workingDir string,
	sessionID string,
	prompt string,
	stdout io.Writer,
) process.Command {
	arguments := []string{"run", "--format", "json", "--dir", workingDir}
	if config.Model != "" {
		arguments = append(arguments, "--model", config.Model)
	}
	if config.Variant != "" {
		arguments = append(arguments, "--variant", config.Variant)
	}
	if sessionID != "" {
		arguments = append(arguments, "--session", sessionID)
	}
	if config.PermissionPolicy == PermissionAutoApprove {
		arguments = append(arguments, "--auto")
	}
	arguments = append(arguments, config.ExtraArgs...)

	return process.Command{
		Name:        config.Executable,
		Args:        arguments,
		Dir:         workingDir,
		Timeout:     config.Timeout,
		Stdin:       strings.NewReader(prompt),
		Stdout:      stdout,
		OutputLimit: process.DefaultOutputLimit,
	}
}
