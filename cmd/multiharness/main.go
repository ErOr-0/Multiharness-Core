// Command multiharness composes the plain-Go workflow with local CLI adapters.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"multiharness-core/internal/adapter/account"
	"multiharness-core/internal/adapter/process"
	"multiharness-core/internal/adapter/setup"
	"multiharness-core/internal/config"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

// Release builds set these values through linker flags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--version" {
		if _, err := fmt.Fprintf(
			stdout,
			"magent %s (commit %s, built %s, %s/%s)\n",
			version, commit, date, runtime.GOOS, runtime.GOARCH,
		); err != nil {
			return cli.ExitFailed
		}
		return cli.ExitSuccess
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	baseDir, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cannot determine invocation directory:", err)
		return cli.ExitUsage
	}
	if len(args) > 0 && args[0] == "context" {
		settingsDir, err := os.UserConfigDir()
		if err != nil {
			return cli.ExitUsage
		}
		return cli.ContextGet(args[1:], baseDir, filepath.Join(settingsDir, "magent", "config.json"), stdout, stderr)
	}
	approver := cli.NewTerminalApprover(os.Stdin, stderr)
	installer := cli.NewTerminalInstaller(os.Stdin, stderr)
	workspaceApprover := cli.NewTerminalWorkspaceApprover(os.Stdin, stderr)
	credentials := &cli.DecisionCredentials{Getenv: os.Getenv, Prompt: cli.NewTerminalDecisionKeyPrompt(os.Stdin, stderr)}
	factory := func(cfg config.Config, events workflow.EventSink) (cli.Runner, error) {
		if cfg.Mode == "direct" {
			return buildDelegation(cfg, events, cli.WithProgressInstallation(installer, events))
		}
		var apiKey string
		if cfg.Decision.Enabled {
			var err error
			apiKey, err = credentials.Resolve(ctx, events)
			if err != nil {
				return nil, err
			}
		}
		dependencies, err := buildDependenciesWithDecisionKey(cfg, events, cli.WithProgressInstallation(installer, events), cli.WithProgressWorkspaceApproval(workspaceApprover, events), apiKey)
		if err != nil {
			return nil, err
		}
		if cfg.Fallback.Mode == "prompt" {
			dependencies.Fallbacks.Approver = cli.WithProgressApproval(cli.FallbackReadiness{Config: cfg, Approver: approver, Check: func(ctx context.Context, r account.Request) account.Status {
				return account.Check(ctx, process.NewOSRunner(), r)
			}, Output: stderr}, events)
		}
		return workflow.NewService(dependencies)
	}
	handler, err := cli.NewHandler(factory, stdout, stderr, baseDir, os.LookupEnv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return cli.ExitFailed
	}
	handler.SetReadiness(func(ctx context.Context, request account.Request) account.Status {
		return account.Check(ctx, process.NewOSRunner(), request)
	}, credentials.CheckSetup)
	handler.SetConfiguredAccountLogin(func(ctx context.Context, request account.Request) error {
		loginArgs := []string{"auth", "login"}
		if request.Harness == "opencode" {
			if provider, _, ok := strings.Cut(request.Model, "/"); ok {
				loginArgs = append(loginArgs, "--provider", provider)
			}
		}
		if request.Harness == "codex" {
			loginArgs = []string{"login", "--device-auth"}
			if os.Getenv("MAGENT_WORKSPACE_ROOT") != "" {
				loginArgs = append([]string{"-c", `cli_auth_credentials_store="file"`}, loginArgs...)
			}
		}
		confirm := installer
		if request.InstallMode != "prompt" {
			confirm = nil
		}
		runner := setup.Runner{Runner: process.NewOSRunner(), Manager: setup.NewManager(process.NewOSRunner(), confirm, 5*time.Minute), Tool: request.Harness}
		_, err := runner.Run(ctx, process.Command{Name: request.Executable, Args: loginArgs, Dir: request.Directory, Stdin: os.Stdin, Stdout: stdout, Stderr: stderr})
		return err
	})
	if len(args) == 0 {
		input, err := cli.NewTerminalInput(os.Stdin, stdout)
		if err != nil {
			return handler.Run(ctx, args)
		}
		settingsDir, err := os.UserConfigDir()
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "cannot locate personal configuration directory")
			return cli.ExitUsage
		}
		return handler.Interactive(ctx, input, filepath.Join(settingsDir, "magent", "config.json"))
	}
	return handler.Run(ctx, args)
}
