package codex

import (
	"strconv"
	"strings"

	"multiharness-core/internal/adapter/process"
)

func buildCommand(
	config Config,
	workingDir string,
	prompt string,
	artifacts invocationArtifacts,
) process.Command {
	arguments := []string{
		"exec",
		"--model", config.Model,
		"--sandbox", string(config.Sandbox),
	}
	if !config.KeepSession {
		arguments = append(arguments, "--ephemeral")
	}
	arguments = append(arguments,
		"--json",
		"--color", "never",
		"--cd", workingDir,
		"--config", "model_reasoning_effort=" + strconv.Quote(config.Reasoning),
		"--output-schema", artifacts.schemaPath,
		"--output-last-message", artifacts.outputPath,
	)
	arguments = append(arguments, config.ExtraArgs...)
	arguments = append(arguments, "-")

	return process.Command{
		Name:        config.Executable,
		Args:        arguments,
		Dir:         workingDir,
		Timeout:     config.Timeout,
		Stdin:       strings.NewReader(prompt),
		OutputLimit: process.DefaultOutputLimit,
	}
}
