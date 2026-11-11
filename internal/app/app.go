// Package app is the command dispatcher: it maps the first argument to the
// package that implements the command.
package app

import (
	"fmt"
	"io"
	"os"

	"multiharness-core/internal/cli"
	"multiharness-core/internal/job"
	"multiharness-core/internal/mcp"
	"multiharness-core/internal/route"
	"multiharness-core/internal/setup"
)

// Run runs one harness-bridge command and returns its exit code.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		if f, ok := stdin.(*os.File); ok && cli.IsTerminal(f) {
			return setup.CmdWizard(stdin, stdout, stderr)
		}
		fmt.Fprint(stderr, cli.Usage)
		return cli.ExitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "mcp":
		return mcp.CmdMCP(rest, stdin, stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, "harness-bridge", cli.Version)
		return cli.ExitOK
	case "start":
		return job.CmdStart(rest, stdin, stdout, stderr)
	case "run":
		return job.CmdRun(rest, stdin, stdout, stderr)
	case "wait":
		return job.CmdWait(rest, stdout, stderr)
	case "cancel":
		return job.CmdCancel(rest, stdout, stderr)
	case "setup":
		return setup.CmdSetup(rest, stdin, stdout, stderr)
	case "disconnect":
		return setup.CmdDisconnect(rest, stdout, stderr)
	case "uninstall":
		return setup.CmdUninstall(rest, stdin, stdout, stderr)
	case "route":
		return route.CmdRoute(rest, stdin, stdout, stderr)
	case job.WorkerCommand:
		return job.CmdWorker(rest, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, cli.Usage)
		return cli.ExitOK
	}
	return cli.UsageError(stdout, stderr, fmt.Errorf("unknown command %q", args[0]))
}
