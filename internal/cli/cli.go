// Package cli holds what every harness-bridge command shares: the exit codes,
// the usage text, the release version and the terminal check.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
)

const (
	ExitOK     = 0
	ExitFailed = 1
	ExitUsage  = 2
)

// Version is the release version; main sets it from the value the release
// build passes with -ldflags "-X main.version=...".
var Version = "dev"

const Usage = `harness-bridge lets one coding agent plan and review while another implements.
Agents: codex (Codex CLI), claude (Claude Code), muse (Muse Code),
opencode (OpenCode), commandcode (Command Code).

Run harness-bridge with no arguments in a terminal to set it up: it asks which
agent plans and which writes code, installs what's missing after you confirm,
and connects itself to the planner for all your projects.

Usage:
  harness-bridge setup  --planner AGENT --implementer AGENT [options]
  harness-bridge disconnect [--planner AGENT | --all]   undo setup for a planner
  harness-bridge uninstall [--yes]                      disconnect everything and remove harness-bridge
  harness-bridge mcp                                    MCP server the planner starts
  harness-bridge start  --to AGENT [options] < prompt   start a background job
  harness-bridge wait   JOB [--max 9m]                  wait; prints progress or the result
  harness-bridge cancel JOB                             stop a job
  harness-bridge run    --to AGENT [options] < prompt   run in the foreground
  harness-bridge route  --provider laya|jev [options] < request

setup installs any missing agent CLI after asking (or with --yes), connects
harness-bridge to the planner for all projects and saves the choices:
  --router laya|jev    also tell the planner to route requests first
  --model NAME         with setup: the implementer's model for every job
  --reasoning LEVEL    with setup: the implementer's reasoning effort for every job
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

func UsageError(stdout, stderr io.Writer, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, Usage)
		return ExitOK
	}
	fmt.Fprintf(stderr, "harness-bridge: %v\nRun 'harness-bridge help' for usage.\n", err)
	return ExitUsage
}
