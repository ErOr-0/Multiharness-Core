package account

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"multiharness-core/internal/adapter/agent/native"
	"multiharness-core/internal/adapter/musecli"
	"multiharness-core/internal/adapter/process"
)

type Model = native.Model

// Claude Code has no catalog command. These are the aliases and full names its
// --model flag accepts; an alias always selects the latest model of that family.
var claudeModels = []Model{
	{ID: "sonnet", Description: "Latest Sonnet - balanced speed and capability", Default: true},
	{ID: "opus", Description: "Latest Opus - most capable for complex work"},
	{ID: "fable", Description: "Latest Fable"},
	{ID: "haiku", Description: "Latest Haiku - fastest and lowest cost"},
	{ID: "claude-fable-5-1", Description: "Fable 5.1"},
	{ID: "claude-opus-5-5", Description: "Opus 5.5"},
	{ID: "claude-sonnet-5-5", Description: "Sonnet 5.5"},
	{ID: "claude-haiku-4-5", Description: "Haiku 4.5"},
}

// Models lists what the selected CLI can run, using metadata queries that never
// submit a prompt. An empty list means nothing can be confirmed as available.
func Models(ctx context.Context, runner Runner, r Request) ([]Model, error) {
	switch r.Harness {
	case "claude":
		return claudeModels, nil
	case "codex":
		return native.CodexModels(ctx, runner, r.Executable, r.Directory)
	case "muse":
		return museModels(ctx, runner, r)
	}
	return nil, errors.New("unsupported agent")
}

func museModels(ctx context.Context, runner Runner, r Request) ([]Model, error) {
	name, err := musecli.Executable(r.Executable)
	if err != nil {
		return nil, errors.New("Muse CLI unavailable")
	}
	// MSP metadata queries do not submit prompts or start agent turns. EOF closes
	// the ephemeral host. Never read/export credentials or expose account data.
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"multiharness","version":"1"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"initialized","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"model/list","params":{}}` + "\n"
	result, err := runner.Run(ctx, process.Command{Name: name, Args: []string{"serve", "--no-session-log", "--disable-write", "--disable-shell"}, Dir: r.Directory, Timeout: 20 * time.Second, Stdin: strings.NewReader(input), OutputLimit: 256 << 10})
	if err != nil || result.ExitCode != 0 || result.StdoutTruncated {
		return nil, errors.New("Muse catalog check failed")
	}
	initialized := false
	for _, line := range strings.Split(result.Stdout, "\n") {
		var response struct {
			ID     int             `json:"id"`
			Error  json.RawMessage `json:"error"`
			Result struct {
				Server struct {
					Name string `json:"name"`
				} `json:"serverInfo"`
				Models []struct {
					ID          string  `json:"modelId"`
					Description *string `json:"description"`
					Default     bool    `json:"isDefault"`
				} `json:"models"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &response) != nil || len(response.Error) > 0 {
			continue
		}
		if response.ID == 1 && response.Result.Server.Name == "muse" {
			initialized = true
		}
		if response.ID == 2 && initialized {
			var models []Model
			for _, m := range response.Result.Models {
				if m.ID == "" {
					continue
				}
				model := Model{ID: m.ID, Default: m.Default}
				if m.Description != nil {
					model.Description = *m.Description
				}
				models = append(models, model)
			}
			return models, nil
		}
	}
	return nil, errors.New("Muse returned no model list")
}
