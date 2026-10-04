// Package account checks native CLI account setup without starting an agent task.
package account

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"multiharness-core/internal/adapter/process"
)

type Request struct{ Harness, Executable, Model, Directory, InstallMode string }
type Status struct {
	Ready  bool
	Detail string
}
type Runner interface {
	Run(context.Context, process.Command) (process.Result, error)
}

// Check never forwards provider output: login status can include account details.
// Native status and model discovery establish local setup, not remote entitlement.
func Check(ctx context.Context, runner Runner, r Request) Status {
	if r.Harness == "muse" {
		return checkMuse(ctx, runner, r)
	}
	args := []string{"login", "status"}
	switch r.Harness {
	case "codex":
	case "claude":
		args = []string{"auth", "status", "--json"}
	default:
		return Status{Detail: "Unsupported agent"}
	}
	result, err := runner.Run(ctx, process.Command{Name: r.Executable, Args: args, Dir: r.Directory, Timeout: 15 * time.Second, OutputLimit: 256 << 10, EnvOverrides: map[string]string{"NO_COLOR": "1"}})
	if err != nil || result.ExitCode != 0 {
		return Status{Detail: "CLI unavailable or account check failed; use /login " + r.Harness + " and /configuration to retry"}
	}
	if result.StdoutTruncated || result.StderrTruncated {
		return Status{Detail: "Account check output exceeded its limit; readiness could not be established"}
	}
	switch r.Harness {
	case "codex":
		// Do not accept an arbitrary successful executable as proof of login.
		if !strings.Contains(result.Stdout+result.Stderr, "Logged in using") {
			return Status{Detail: "Codex login is missing or unrecognized; use /login codex"}
		}
		return Status{true, "CLI reports signed in"}
	case "claude":
		var status struct {
			LoggedIn bool `json:"loggedIn"`
		}
		if json.Unmarshal([]byte(result.Stdout), &status) != nil || !status.LoggedIn {
			return Status{Detail: "Claude login is missing or unrecognized; use /login claude"}
		}
		return Status{true, "CLI reports signed in"}
	}
	return Status{Detail: "Account status unknown"}
}
