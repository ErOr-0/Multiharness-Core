// Command harness-bridge lets one coding agent hand work to another agent CLI
// and read back a compact JSON report.
//
// The outer agent (for example Claude Code) plans and reviews. It calls
// harness-bridge, which runs the inner agent CLI (Codex CLI, Claude Code, Muse
// Code, OpenCode or Command Code) in headless mode, follows its event stream
// and reports the final message, the session ID for follow-up fixes, the files
// the agent touched and what Git sees in the workspace afterwards. setup
// installs the CLIs and writes the planner's instructions; route asks a Jev or
// Laya decision model whether a request needs a plan at all.
//
// Implementations often outlast a single shell call, so work runs as a job:
//
//	harness-bridge start --to codex < plan.md   # returns at once with a job ID
//	harness-bridge wait JOB --max 9m            # repeat while status is "running"
//	harness-bridge cancel JOB
//
// harness-bridge run takes the same options as start and blocks until the job
// ends.
package main

import (
	"os"

	"multiharness-core/internal/app"
	"multiharness-core/internal/cli"
)

// version is set at release build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cli.Version = version
	os.Exit(app.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
