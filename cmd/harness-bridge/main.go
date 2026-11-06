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
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

const (
	statusRunning         = "running"
	statusCompleted       = "completed"
	statusFailed          = "failed"
	statusNeedsPermission = "needs_permission"
	statusTimedOut        = "timed_out"
	statusCancelled       = "cancelled"
	statusLost            = "lost"
)

const usage = `harness-bridge lets one coding agent plan and review while another implements.
Agents: codex (Codex CLI), claude (Claude Code), muse (Muse Code),
opencode (OpenCode), commandcode (Command Code).

Run harness-bridge with no arguments in a terminal to set it up: it asks which
agent plans and which writes code, installs what's missing after you confirm,
and connects itself to the planner for all your projects.

Usage:
  harness-bridge setup  --planner AGENT --implementer AGENT [options]
  harness-bridge mcp                                    MCP server the planner starts
  harness-bridge start  --to AGENT [options] < prompt   start a background job
  harness-bridge wait   JOB [--max 9m]                  wait; prints progress or the result
  harness-bridge cancel JOB                             stop a job
  harness-bridge run    --to AGENT [options] < prompt   run in the foreground
  harness-bridge route  --provider laya|jev [options] < request

setup installs any missing agent CLI after asking (or with --yes), connects
harness-bridge to the planner for all projects and saves the choices:
  --router laya|jev    also tell the planner to route requests first
  --yes                run installers without asking (only after the user agreed)
  --dir PROJECT        instead, write instruction files into that project, for
                       planners that can't load MCP servers
  --lock-planner       with --dir: stop a Claude Code planner editing files

Options for start and run:
  --to AGENT           agent that implements (required)
  --dir DIR            workspace the agent works in (default .)
  --session ID         continue an earlier session, for follow-up fixes
  --sandbox MODE       workspace-write (default) or read-only
  --model NAME         model for the agent
  --reasoning LEVEL    reasoning effort for the agent
  --allow RULE         extra Claude Code tool rule, e.g. 'Bash(go test:*)' (repeatable)
  --timeout DURATION   stop the agent after this long (default 60m)
  --prompt TEXT        prompt text instead of stdin
  --prompt-file FILE   read the prompt from FILE instead of stdin
  --bin PATH           agent executable (default: its usual name on PATH)
  -- ARGS...           pass the remaining arguments to the agent CLI unchanged

Job status is running, completed, failed, needs_permission, timed_out,
cancelled or lost. The exit status is 0 for running and completed, 1 otherwise,
and 2 for usage errors. Jobs are kept for 7 days in HARNESS_BRIDGE_HOME
(default: the user cache directory).

route asks a decision model whether a request needs a plan (or, with --review,
whether a change needs a full review). Laya reads LAYA_ENDPOINT (default
http://127.0.0.1:8765/v1/systemone), LAYA_MODEL and LAYA_API_KEY; Jev needs
OPENROUTER_API_KEY. --provider defaults to HARNESS_BRIDGE_ROUTER. Options:
--model, --endpoint, --timeout (10s), --threshold (0.75), --prompt, --prompt-file.
When the model is unreachable or unsure, route answers needs_planning
(or needs_full_review) and says why.
`

func main() { os.Exit(cli(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func cli(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		if f, ok := stdin.(*os.File); ok && isTerminal(f) {
			return cmdWizard(stdin, stdout, stderr)
		}
		fmt.Fprint(stderr, usage)
		return exitUsage
	}
	rest := args[1:]
	switch args[0] {
	case "mcp":
		return cmdMCP(rest, stdin, stdout, stderr)
	case "version", "--version":
		fmt.Fprintln(stdout, "harness-bridge", version)
		return exitOK
	case "start":
		return cmdStart(rest, stdin, stdout, stderr)
	case "run":
		return cmdRun(rest, stdin, stdout, stderr)
	case "wait":
		return cmdWait(rest, stdout, stderr)
	case "cancel":
		return cmdCancel(rest, stdout, stderr)
	case "setup":
		return cmdSetup(rest, stdin, stdout, stderr)
	case "route":
		return cmdRoute(rest, stdin, stdout, stderr)
	case workerCommand:
		return cmdWorker(rest, stderr)
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	return usageError(stdout, stderr, fmt.Errorf("unknown command %q", args[0]))
}

func usageError(stdout, stderr io.Writer, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	fmt.Fprintf(stderr, "harness-bridge: %v\nRun 'harness-bridge help' for usage.\n", err)
	return exitUsage
}

func printReport(w io.Writer, r Report) int {
	r.decorate()
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false) // agents read this; keep & < > in commands readable
	enc.SetIndent("", "  ")
	if err := enc.Encode(r); err != nil {
		fmt.Fprintf(w, "{\"status\":%q,\"error\":%q}\n", statusFailed, "cannot encode the report: "+err.Error())
		return exitFailed
	}
	_, _ = w.Write(out.Bytes())
	if r.Status == statusRunning || r.Status == statusCompleted {
		return exitOK
	}
	return exitFailed
}
