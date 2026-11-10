# Multiharness Core: `harness-bridge`

**One coding agent plans and reviews; another writes the code.** For example,
Claude Code plans, Codex CLI implements, and Claude Code reviews the result.
`harness-bridge` connects the two. Any supported pairing works.

## Install

**Windows** (PowerShell):

```powershell
irm https://raw.githubusercontent.com/ErOr-0/Multiharness-Core/main/install.ps1 | iex
```

**macOS or Linux**:

```sh
curl -fsSL https://raw.githubusercontent.com/ErOr-0/Multiharness-Core/main/install.sh | sh
```

The script downloads the latest release for your system, checks its checksum
and starts the setup. Setup asks three questions:

```text
Which agent should plan and review your work?
  1) Codex CLI     will be installed
  2) Claude Code   installed
  …
Which agent should write the code?
Before planning, ask a decision model whether a request needs a plan at all?

Here's what happens next:
  • Install Codex CLI:  npm install --global @openai/codex
  • Connect harness-bridge to Claude Code for all your projects
  • Let Claude Code use harness-bridge's tools without asking each time
  • Sign in to any agent that isn't signed in yet (your browser opens)
  • Save these choices (run harness-bridge again to change them)
Continue? [Y/n]
```

After one confirmation, the setup finishes on its own:
- it installs missing agents in the background, with a progress line;
- it opens each agent's sign-in, the one step that needs you;
- it connects itself to the planner.

From then on there is nothing to run. Open your planner in any project (a new
session, since planners load MCP servers when a session starts) and ask for a
change. You type requests in the planner, not in harness-bridge.

To change your choices later, run `harness-bridge` again.

## Turning it off

```sh
harness-bridge disconnect            # undo setup for your planner
harness-bridge disconnect --all      # every planner it was connected to
harness-bridge uninstall             # disconnect everything and remove harness-bridge
```

`disconnect` removes what setup added to the planner:
- the MCP server registration;
- Claude Code's allow rule;
- the instructions block in Claude Code's global `CLAUDE.md` or in OpenCode's or Command Code's global `AGENTS.md`.

The planner then works as it did before; run `harness-bridge` to connect again.

`uninstall` asks once, then also deletes your saved choices, the job logs, the
program and, on Windows, its PATH entry. Your agent CLIs stay installed.

## How it works

Setup registers `harness-bridge mcp` as an MCP server with the planner, once,
for all projects. For Claude Code that is `claude mcp add --scope user`, and it
writes its instructions into the global `~/.claude/CLAUDE.md`. The
planner starts the server itself whenever it runs. The server gives the
planner five tools, and its instructions tell the planner how to use them:

| Tool | What it does |
| ---- | ------------ |
| `delegate` | Starts the implementer on a plan, in the background, and returns a job id |
| `wait`     | Returns the result, or the latest progress while the job runs |
| `cancel`   | Stops a job |
| `route`    | Asks Jev or Laya whether a request needs a plan at all |
| `agents`   | Lists the agents, whether each is installed, and their roles |

The implementer runs headless, with the plan on stdin. The result says:
- the job's `status`;
- the `session_id`, for sending fixes;
- the agent's final `message`;
- the files and commands it reported;
- any actions it was refused;
- `git`: what `git status` and `git diff --stat` show afterwards.

The planner reviews the repository itself, not just the report, and sends
fixes to the same session.

## Supported agents

| Agent        | Plans (MCP) | Implements | Continues a session | Notes |
| ------------ | ----------- | ---------- | ------------------- | ----- |
| Claude Code  | yes | yes | yes | Gets its instructions through `~/.claude/CLAUDE.md`. Runs shell commands only through `allow` rules such as `Bash(go test:*)` |
| Codex CLI    | yes | yes | yes | Commands have no network unless you allow it |
| OpenCode     | yes | yes | yes | v1 and v2; gets its instructions through `~/.config/opencode/AGENTS.md`. Headless it rejects permission prompts; see `opencode.json` or `--auto` |
| Command Code | yes | yes | yes | Gets its instructions through `~/.commandcode/AGENTS.md`. Version 1.88 edits files headless only with `--yolo`, which also allows any shell command, so a job without it ends `needs_permission` and the planner asks you first |
| Muse Code    | no  | yes | no  | No shell headless, so the planner runs the checks |

Claude Code can't implement for itself, because it refuses to start inside
another Claude Code session; use its own subagents for that. Each agent is one
adapter file in [cmd/harness-bridge](cmd/harness-bridge), for example
[codex.go](cmd/harness-bridge/codex.go). The file holds the agent's command
line, its event parser, its installer and its sign-in check.

## Routing with Jev or Laya

If you choose a router in setup, the planner first asks it whether a request
needs a plan. The answer is `answer`, `needs_planning` or `direct_implement`.

- **Laya** is self-hosted. It reads `LAYA_ENDPOINT` (default
  `http://127.0.0.1:8765/v1/systemone`); `LAYA_MODEL` and `LAYA_API_KEY` are
  optional.
- **Jev** is reached through OpenRouter and needs `OPENROUTER_API_KEY`.

When the model is unreachable or unsure (below 0.75 confidence), it returns
the safe choice and says why. From a shell:

```sh
echo "fix the typo in README.md" | harness-bridge route --provider laya
```

## Command line

Everything the MCP tools do is also available from a shell, for scripts and for
planners without MCP:

```sh
harness-bridge start < plan.md        # the implementer chosen in setup; prints a job id
harness-bridge wait JOB --max 9m      # repeat while status is "running"
harness-bridge start --session ID < fixes.md
harness-bridge cancel JOB
harness-bridge run --to opencode < plan.md   # block until it ends
harness-bridge setup --planner claude --implementer codex --yes   # setup without questions
harness-bridge setup --planner muse --implementer codex --dir PROJECT   # instruction files instead of MCP
```

Run `harness-bridge help` for every option. A few places to know:
- **Choices:** saved in `harness-bridge/config.json` in your user config
  folder.
- **Jobs and their logs:** kept for 7 days in the user cache folder.
- **Install logs:** written to the temp folder.

## What it does not enforce

The workflow is instructions to the planner, so a planner can skip a step. The
bridge itself guarantees:
- timeouts and cancellation that stop the whole process tree;
- a report with independent Git evidence for every job;
- no prompt ever passes through a shell;
- installs only after you confirm, in setup.

The MCP tools can't install anything.

## Releasing

Push a version tag, for example `git tag v0.1.0 && git push origin v0.1.0`.
The [release workflow](.github/workflows/release.yml) runs the checks, builds
Windows, macOS and Linux binaries for amd64 and arm64, and publishes them with
`checksums.txt`. The install scripts take the latest release. The binaries are
not code-signed, so Windows SmartScreen or macOS Gatekeeper may warn the first
time.

To build from source instead:

```sh
go install ./cmd/harness-bridge
```

## Development

```sh
make check            # gofmt, vet for Linux/macOS/Windows, build, tests, race tests
make lint security lint-workflows
```

The tests drive every agent, the MCP server, the wizard and the setup against
fake CLIs. They don't run real agents.
