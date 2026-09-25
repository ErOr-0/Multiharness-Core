package account

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"multiharness-core/internal/adapter/musecli"
	"multiharness-core/internal/adapter/process"
)

func checkMuse(ctx context.Context, runner Runner, r Request) Status {
	name, err := musecli.Executable(r.Executable)
	if err != nil {
		return Status{Detail: "Muse CLI unavailable; install Muse Code and run /login muse"}
	}
	// MSP metadata queries do not submit prompts or start agent turns. EOF closes
	// the ephemeral host. Never read/export credentials or expose account data.
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"multiharness","version":"1"}}}` + "\n" +
		`{"jsonrpc":"2.0","method":"initialized","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","id":2,"method":"model/list","params":{}}` + "\n"
	result, err := runner.Run(ctx, process.Command{Name: name, Args: []string{"serve", "--no-session-log", "--disable-write", "--disable-shell"}, Dir: r.Directory, Timeout: 20 * time.Second, Stdin: strings.NewReader(input), OutputLimit: 256 << 10})
	if err != nil || result.ExitCode != 0 || result.StdoutTruncated {
		return Status{Detail: "Muse account/catalog check failed; run /login muse and retry /configuration"}
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
					ID string `json:"modelId"`
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
			for _, model := range response.Result.Models {
				if model.ID == r.Model {
					return Status{true, "Model listed by Muse CLI; subscription and remaining allowance are checked when used"}
				}
			}
		}
	}
	return Status{Detail: "Selected model is not listed by Muse; run muse /models, then update /config"}
}
