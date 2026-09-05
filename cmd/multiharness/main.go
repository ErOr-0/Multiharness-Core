// Command multiharness composes the plain-Go workflow with local CLI adapters.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"multiharness-core/internal/config"
	"multiharness-core/internal/transport/cli"
	"multiharness-core/internal/workflow"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	baseDir, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "cannot determine invocation directory:", err)
		return cli.ExitUsage
	}
	approver := cli.NewTerminalApprover(os.Stdin, stderr)
	installer := cli.NewTerminalInstaller(os.Stdin, stderr)
	factory := func(cfg config.Config, events workflow.EventSink) (cli.Runner, error) {
		dependencies, err := buildDependenciesWithInstallation(cfg, events, cli.WithProgressInstallation(installer, events))
		if err != nil {
			return nil, err
		}
		if cfg.Fallback.Mode == "prompt" {
			dependencies.Fallbacks.Approver = cli.WithProgressApproval(approver, events)
		}
		return workflow.NewService(dependencies)
	}
	handler, err := cli.NewHandler(factory, stdout, stderr, baseDir, os.LookupEnv)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return cli.ExitFailed
	}
	return handler.Run(ctx, args)
}
