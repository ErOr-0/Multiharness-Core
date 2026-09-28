// Package directexec translates native CLI protocols into direct responses.
// It never decides workflow policy or asks the model for a Multiharness schema.
package directexec

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"multiharness-core/internal/adapter/agent/native"
	"multiharness-core/internal/adapter/agent/provider"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/store"
)

type Runner interface {
	Run(context.Context, process.Command) (process.Result, error)
}

// Config is adapter-owned; the composition root maps application settings here.
type Config struct {
	Approver                                                                  store.NativeApprover
	Harness, Executable, Model, Reasoning, Variant, PermissionPolicy, Sandbox string
	ExtraArgs                                                                 []string
}

type Agent struct {
	runner Runner
	config Config
}

func New(runner Runner, cfg Config) (*Agent, error) {
	if runner == nil || strings.TrimSpace(cfg.Executable) == "" {
		return nil, errors.New("direct adapter requires a runner and executable")
	}
	if cfg.Harness != "codex" && cfg.Harness != "opencode" && cfg.Harness != "claude" {
		return nil, errors.New("unsupported direct agent")
	}
	if err := normalizePermissions(&cfg); err != nil {
		return nil, err
	}
	// Resume selectors and CLI policy are owned by this adapter. Existing role
	// validation also rejects provider-specific managed flags at composition.
	for _, arg := range cfg.ExtraArgs {
		name, _, _ := strings.Cut(arg, "=")
		switch name {
		case "--last", "--all", "--resume", "--session-id", "--continue", "--fork-session":
			return nil, fmt.Errorf("direct adapter manages %s", name)
		}
	}
	cfg.ExtraArgs = append([]string(nil), cfg.ExtraArgs...)
	return &Agent{runner: runner, config: cfg}, nil
}

func (a *Agent) command(input store.TaskInput) process.Command {
	c := a.config
	var args []string
	switch c.Harness {
	case "codex":
		// Config flags work on both exec and exec resume; resume has no --sandbox/--cd.
		args = []string{"exec"}
		if input.SessionID != "" {
			args = append(args, "resume")
		}
		args = append(args, "--json", "--skip-git-repo-check", "-c", "sandbox_mode="+strconv.Quote(c.Sandbox), "-c", `approval_policy="never"`)
		if c.Model != "" {
			args = append(args, "--model", c.Model)
		}
		if c.Reasoning != "" {
			args = append(args, "-c", "model_reasoning_effort="+strconv.Quote(c.Reasoning))
		}
		args = append(args, c.ExtraArgs...)
		if input.SessionID != "" {
			args = append(args, input.SessionID)
		}
		args = append(args, "-")
	case "opencode":
		args = []string{"run", "--format", "json", "--dir", input.WorkingDir}
		if c.Model != "" {
			args = append(args, "--model", c.Model)
		}
		if c.Variant != "" {
			args = append(args, "--variant", c.Variant)
		}
		if input.SessionID != "" {
			args = append(args, "--session", input.SessionID)
		}
		if c.PermissionPolicy == "auto_approve" {
			args = append(args, "--auto")
		}
		args = append(args, c.ExtraArgs...)
	case "claude":
		args = []string{"--print", "--output-format", "stream-json", "--verbose", "--permission-mode", claudePermissionMode(c.PermissionPolicy)}
		if c.Model != "" {
			args = append(args, "--model", c.Model)
		}
		if c.Reasoning != "" {
			args = append(args, "--effort", c.Reasoning)
		}
		if input.SessionID != "" {
			args = append(args, "--resume", input.SessionID)
		}
		// Keep the existing write-tool allowance. Native user permission rules still
		// decide whether shell/MCP tools may run under the user's selected mode.
		args = append(args, "--allowedTools", "Read,Glob,Grep,Edit,Write")
		args = append(args, c.ExtraArgs...)
	}
	return process.Command{Name: c.Executable, Args: args, Dir: input.WorkingDir, Stdin: strings.NewReader(input.Task), OutputLimit: 4 << 20}
}

func (a *Agent) Execute(ctx context.Context, input store.TaskInput) (store.DirectResponse, error) {
	if a.config.Approver != nil && a.config.Harness == "opencode" && a.config.PermissionPolicy == "reject_on_prompt" {
		cfg := a.config
		if len(cfg.ExtraArgs) > 0 {
			return store.DirectResponse{}, errors.New("OpenCode extra_args are not supported with live approvals; use explicit settings")
		}
		response, err := native.OpenCode(ctx, a.runner, native.Config{Executable: cfg.Executable, Model: cfg.Model, Variant: cfg.Variant, CanWrite: true, Direct: true, Approver: cfg.Approver}, native.Request{Directory: input.WorkingDir, Prompt: input.Task, SessionID: input.SessionID})
		return store.DirectResponse{Text: response.Text, SessionID: response.SessionID}, err
	}
	if a.config.Approver != nil && (a.config.Harness == "codex" || a.config.Harness == "claude") {
		if len(a.config.ExtraArgs) > 0 {
			return store.DirectResponse{}, errors.New("extra_args are not supported with live approvals; use explicit settings")
		}
		c := a.config
		cfg := native.Config{Executable: c.Executable, Model: c.Model, Reasoning: c.Reasoning, Sandbox: c.Sandbox, CanWrite: c.Sandbox != "read-only", Direct: true, Approver: c.Approver}
		if c.Harness == "claude" && c.PermissionPolicy != "reject_on_prompt" {
			cfg.PermissionMode = claudePermissionMode(c.PermissionPolicy)
		}
		req := native.Request{Directory: input.WorkingDir, Prompt: input.Task, SessionID: input.SessionID}
		var response native.Response
		var err error
		if c.Harness == "codex" {
			response, err = native.Codex(ctx, a.runner, cfg, req)
		} else {
			response, err = native.Claude(ctx, a.runner, cfg, req)
		}
		return store.DirectResponse{Text: response.Text, SessionID: response.SessionID}, err
	}
	stream := newStream(a.config.Harness, input.SessionID)
	command := a.command(input)
	command.Stdout = stream
	result, runErr := provider.Run(ctx, a.runner, command)
	response, parseErr := stream.finish()
	if runErr != nil {
		return response, runErr
	}
	if result.ExitCode != 0 {
		return response, errors.New("CLI exited unsuccessfully")
	}
	return response, parseErr
}
