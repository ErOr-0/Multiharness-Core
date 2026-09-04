package codex

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxFinalOutputBytes = 4 * 1024 * 1024

type invocationArtifacts struct {
	directory  string
	schemaPath string
	outputPath string
}

func createInvocationArtifacts(role string, schema []byte) (invocationArtifacts, error) {
	directory, err := os.MkdirTemp("", "multiharness-codex-")
	if err != nil {
		return invocationArtifacts{}, fmt.Errorf("create temporary directory: %w", err)
	}

	artifacts := invocationArtifacts{
		directory:  directory,
		schemaPath: filepath.Join(directory, role+".schema.json"),
		outputPath: filepath.Join(directory, role+".output.json"),
	}
	if err := os.WriteFile(artifacts.schemaPath, schema, 0o600); err != nil {
		artifacts.cleanup()
		return invocationArtifacts{}, fmt.Errorf("write output schema: %w", err)
	}
	if err := os.WriteFile(artifacts.outputPath, nil, 0o600); err != nil {
		artifacts.cleanup()
		return invocationArtifacts{}, fmt.Errorf("create output file: %w", err)
	}
	return artifacts, nil
}

func (artifacts invocationArtifacts) readOutput() ([]byte, error) {
	file, err := os.Open(artifacts.outputPath)
	if err != nil {
		return nil, fmt.Errorf("open final response: %w", err)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxFinalOutputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read final response: %w", err)
	}
	if len(data) > maxFinalOutputBytes {
		return nil, fmt.Errorf("final response exceeds %d bytes", maxFinalOutputBytes)
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, fmt.Errorf("final response is blank")
	}
	return data, nil
}

func (artifacts invocationArtifacts) cleanup() {
	if artifacts.directory != "" {
		_ = os.RemoveAll(artifacts.directory)
	}
}
