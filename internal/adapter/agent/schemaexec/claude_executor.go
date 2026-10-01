package schemaexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"multiharness-core/internal/adapter/agent/native"
	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/agent/structured"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

func (a *Claude) execute(ctx context.Context, dir, prompt string, schema []byte) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("Claude context must not be nil")
	}
	if a.config.Approver != nil {
		cfg := a.config
		response, err := native.Claude(ctx, a.runner, native.Config{Executable: cfg.Executable, Model: cfg.Model, Reasoning: cfg.Effort, Timeout: cfg.Timeout, CanWrite: cfg.CanWrite, Approver: cfg.Approver}, native.Request{Directory: dir, Prompt: prompt, Schema: schema})
		return response.Data, err
	}
	toolset := "Read,Glob,Grep"
	if a.config.CanWrite {
		toolset += ",Edit,Write"
	}
	args := []string{
		"--print",
		"--output-format", "json",
		"--model", a.config.Model,
		"--json-schema", string(schema), "--no-session-persistence", "--permission-mode", "dontAsk",
		"--tools", toolset, "--allowedTools", toolset, "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`,
		"--setting-sources", "user", "--settings", `{"disableAllHooks":true}`, "--disable-slash-commands",
	}
	if a.config.Effort != "" {
		args = append(args, "--effort", a.config.Effort)
	}
	prompt += "\nUse the supplied repository and validation evidence with Read/Glob/Grep to inspect current files. Shell, MCP, subagents and hooks are unavailable. Implementation can use Edit/Write; configured validation runs separately. Never claim unavailable commands ran."
	result, err := provider.Run(ctx, a.runner, process.Command{
		Name: a.config.Executable, Args: args, Dir: dir,
		Timeout: a.config.Timeout, Stdin: strings.NewReader(prompt),
		OutputLimit: 4 * 1024 * 1024,
	})
	if err != nil {
		// Claude reports provider failures (expired login, billing, context)
		// in its result envelope and then exits 1; keep that diagnosis.
		if failure := envelopeFailure(result.Stdout); failure != nil {
			return nil, failure
		}
		return nil, fmt.Errorf("execute Claude: %w", err)
	}
	if result.ExitCode != 0 || result.StdoutTruncated {
		return nil, errors.New("Claude failed or exceeded the output limit")
	}
	data := []byte(result.Stdout)
	if err := structured.ValidateObject(data, "type", "subtype", "is_error", "structured_output", "result", "permission_denials"); err != nil {
		return nil, &structured.OutputError{Role: "Claude", Cause: err}
	}
	var envelope struct {
		Type    string            `json:"type"`
		Subtype string            `json:"subtype"`
		IsError *bool             `json:"is_error"`
		Output  json.RawMessage   `json:"structured_output"`
		Result  string            `json:"result"`
		Denials []json.RawMessage `json:"permission_denials"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return nil, errors.New("invalid Claude result envelope")
	}
	if envelope.IsError != nil && *envelope.IsError {
		if failure := provider.Text(envelope.Result); failure != nil {
			return nil, failure
		}
		return nil, &store.ProviderFailure{Kind: store.ProviderUnknown, Source: "error", Attempts: 1}
	}
	if envelope.Type != "result" || envelope.Subtype != "success" || envelope.IsError == nil || len(envelope.Denials) > 0 || len(envelope.Output) == 0 {
		if envelope.Type == "result" && envelope.Subtype == "success" && envelope.IsError != nil && !*envelope.IsError && len(envelope.Denials) > 0 {
			var denial struct {
				Tool  string `json:"tool_name"`
				Input struct {
					FilePath string `json:"file_path"`
				} `json:"tool_input"`
			}
			if structured.ValidateObject(envelope.Denials[0], "tool_name", "tool_input") == nil && json.Unmarshal(envelope.Denials[0], &denial) == nil {
				blocked := &store.PermissionDenied{Action: store.BlockedAction{Tool: denial.Tool, Target: denial.Input.FilePath}}
				if blocked.Validate() == nil {
					return nil, blocked
				}
			}
		}
		return nil, errors.New("Claude did not return a successful structured result or a required tool was denied")
	}
	return envelope.Output, nil
}

func envelopeFailure(stdout string) *store.ProviderFailure {
	var envelope struct {
		Type    string `json:"type"`
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(stdout)), &envelope) != nil || envelope.Type != "result" || !envelope.IsError {
		return nil
	}
	return provider.Text(envelope.Result)
}
