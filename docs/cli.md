# Local CLI

The CLI runs the plain-Go workflow service with Codex planning/review, OpenCode
implementation/repair, Git evidence, and configured deterministic checks.
The CLI and repair loop are covered by deterministic tests with fake agents.
Opt-in authenticated-agent tests and their current verification status are
documented in [testing.md](testing.md).

## Build and run

Use the Go toolchain specified in `go.mod`. Install and authenticate Codex and
OpenCode separately, and have Git available on `PATH`. The CLI never installs
agents, chooses credentials, or retries authentication automatically.

```sh
go build -o ./bin/multiharness ./cmd/multiharness
./bin/multiharness --help
./bin/multiharness --config examples/multiharness.json \
  --workdir /absolute/path/to/target-repository \
  --task "Implement the requested change and add regression tests."
```

The example checks are for a Go repository. Change `validation.checks` for other
projects. Set the OpenCode `model` to your configured `provider/model`, or leave
it empty to use OpenCode's own default; `variant` behaves similarly.

To avoid putting task text in shell history/process arguments, use a regular
UTF-8 file:

```sh
./bin/multiharness --config examples/multiharness.json \
  --workdir /absolute/path/to/target-repository --task-file task.txt
```

Exactly one task source is required: `--task`, `--task-file`, or one quoted
positional argument. Put flags before positional task text. Stdin is not a task
source in this version; `--task-file -` is rejected. Task files are bounded by
`max_task_bytes`, default 1 MiB. Do not place secrets in task text or configuration
unless you intend the selected agent to receive them.

## Automatic Codex runtime recovery

The default executable name `codex` enables local recovery before each Codex
planning, review, implementation or repair invocation. A newer desktop app can
write `models_cache.json` with values that an older terminal CLI cannot decode.
The runtime reads only the cache's writer version and requires a stable CLI
release at least that new, plus the flags used by our adapter. This conservative
version floor avoids guessing which older releases can parse a newer catalog;
it is not a complete model-access or protocol compatibility guarantee.

The existing PATH candidate is preferred if compatible. Otherwise discovery
checks other absolute PATH entries and, on macOS, Codex.app/ChatGPT.app in
`/Applications` and the user's Applications directory. Symlink duplicates,
repository-local candidates, relative PATH entries, non-executables and
world-writable files are excluded. PATH and installed applications remain an
operator trust boundary; this is not executable signature verification.

Only offline `--version` and `exec --help` probes run during selection. They do
not receive the task or call models. Discovery considers at most eight binaries,
with a two-second probe timeout and ten-second preflight budget, also bounded by
the configured agent/whole-run deadline. These additional fixed defensive caps
prevent discovery from consuming an entire long-running task budget. An explicit
executable path or custom command name opts out of automatic discovery, including
these probes, for managed installations and prerelease builds.

The selected executable receives the original model, reasoning, sandbox, argv,
stdin and environment. The task is launched once: runtime recovery never replays
a task after it starts, resets invocation accounting, or triggers billing fallback.
Logs report `code=codex_runtime_selected` and `runtime_version` (text: `version`),
without exposing local paths, raw diagnostics or cache contents. Quiet mode hides
the notice. A log-write failure prevents the task from starting.

If no compatible installation exists or cache metadata cannot be read safely,
execution stops with an actionable compatibility error before a model request.
There is no automatic download, package-manager invocation, cache rewrite,
credential copy, global settings change, or silent model substitution. Installing
a managed runtime automatically is a separate distribution/update-policy feature,
not implemented by this local recovery. No configuration/wire version changed.

## Configuration precedence

Settings resolve in this order, with later layers winning:

1. Built-in defaults.
2. An explicit JSON file selected with `--config` or `MULTIHARNESS_CONFIG`.
3. Supported `MULTIHARNESS_*` environment variables.
4. Explicit command-line flags.

No configuration is automatically loaded from a target repository. A config
file must declare `"version": 1`. Unknown properties, duplicate keys (including
case-only variants), invalid UTF-8, null values,
invalid types/durations, and unsupported versions fail before any agent runs.
An explicitly selected missing file is an error. An invalid selected file is
not repaired by a later environment/flag override.

Each setting listed by `--help` has a matching environment variable: uppercase
the flag name, replace hyphens with underscores, and prefix `MULTIHARNESS_`.
For example, `--planner-model` maps to `MULTIHARNESS_PLANNER_MODEL`, and
`--workdir` maps to `MULTIHARNESS_WORKDIR`. `--task`, `--task-file`, `--quiet`, and
`--help` are CLI-only. `--config` takes precedence over `MULTIHARNESS_CONFIG`.

```sh
MULTIHARNESS_MAX_REPAIR_ATTEMPTS=2 ./bin/multiharness \
  --config examples/multiharness.json --max-repair-attempts 0 \
  --task "Explain the workflow without changing files."
```

This example permits zero repairs because the flag overrides the environment.
Explicit empty values also override lower layers: an empty implementer model
selects OpenCode's default. Required fields such as the planner model cannot be
cleared. Collections are replaced as a whole with JSON values, for example
`--planner-extra-args '[]'` or `--validation-checks '[]'`.

All relative application paths (config file, working directory, task file, and
explicit agent/Git executable paths) use the CLI's invocation directory, not
the config file's location. Explicit relative validation executables such as
`./scripts/check.sh` use the target directory. Bare executable names use `PATH`
when that stage invokes them; missing executables become structured failures.
Availability and authentication are not probed by running agents at startup, so
an answer-only task does not require OpenCode to be installed.

## Execution and permissions

The planner emits a version-2 decision:

- `implement`: a plan with steps and acceptance criteria, followed by
  implementation → validation → review → repair when needed.
- `answer`: a direct response, with no OpenCode invocation, deterministic checks,
  or separate review. An answer can ask for clarification; it is not approval of
  a code change. An ambiguous or malformed decision fails closed.

Both paths currently require an accessible Git repository root and the same
workspace safety checks. A plain non-repository chatbot mode is not provided.
The workspace must remain unchanged during an answer-only run.

Planning and review are restricted to `read-only` through application
configuration. Extra arguments cannot replace managed model, prompt, directory,
sandbox, schema, session, or permission flags. OpenCode defaults to
`reject_on_prompt`: permissions that require interactive approval are rejected,
not left waiting. `--implementer-permission-policy auto_approve` is an explicit
opt-in to broader implementation-agent permissions. Review the target's agent
configuration before enabling it; configured deny rules still apply.

Pre-existing dirty paths are protected at whole-file granularity. Resolve those
changes yourself before asking the agent to edit the same files. Workspace locks
are cooperative, not a sandbox. Supported Unix platforms are macOS, Linux,
FreeBSD, OpenBSD, NetBSD, and DragonFly BSD. Unsupported repository layouts and
platforms fail explicitly; see the Git adapter's package documentation.

Validation commands use direct executable/argument arrays, never an implicit
shell. Configure a shell explicitly only when needed. Each check can override
`timeout` and `env_overrides`; omitted or `"0s"` check timeouts inherit
`validation.default_timeout`. Application and agent timeouts must be positive.
Checks must not modify captured repository files; ignored build artifacts are
outside the evidence boundary. Output is bounded and its truncation is recorded.
Ordinary failed checks become review evidence; infrastructure errors stop the
workflow. Validation and Git commands are not automatically retried. Explicitly
enabled provider retries only apply to read-only planning/review; see below.

The default check list is empty because the tool cannot infer a project's test
commands safely. The CLI warns when validation starts with no configured checks
(unless `--quiet` is set). An empty list means no tests ran, not that a test suite
passed. Configure real checks before relying on a coding approval.

`max_repair_attempts` counts repair calls, not reviews. The initial implementation
gets one review even when the limit is zero. Reaching a limit never means success.
The default whole-run timeout is four hours, with per-stage limits as shown in
the example. Ctrl+C and SIGTERM propagate cancellation to subprocesses and release
the workspace lease. Interrupted-run resume is not implemented.

### Provider failures and invocation limits

`execution` settings use normal file/environment/flag precedence:

| JSON field | CLI flag | Default |
| --- | --- | --- |
| `max_agent_invocations` | `--max-agent-invocations` | 64 |
| `max_retries` | `--provider-max-retries` | 0 (off) |
| `initial_delay` | `--provider-initial-delay` | `1s` |
| `max_delay` | `--provider-max-delay` | `30s` |
| `max_cost_microusd` | `--max-cost-microusd` | 0 (no monetary cap) |

The launch limit includes every planning, implementation, review, repair, and
retry invocation across one run. It does not count hidden HTTP/model calls made
inside a CLI. Exhaustion never produces approval. Transient retries are opt-in,
bounded, and limited to unchanged read-only stages. Billing, authentication,
access and unclassified errors are not retried; implementation/repair are never
automatically replayed. See [provider-failures.md](provider-failures.md) for exact
backoff, Retry-After, failure metadata, and recovery semantics.

The monetary field is a capability guard, **not an implemented budget**: every
nonzero value fails configuration before starting an agent. A CLI adapter cannot
enforce an authoritative cost cap; use externally enforced billing controls or a
metered gateway if that is required. An invocation limit is not a substitute.

### Confirmed billing fallback

Default `fallback.mode` is `prompt`. When a recognized billing/usage-limit error
occurs and stdin is a terminal, the CLI asks on stderr whether to continue with
the alternate agent. It names the failed role, alternate model and permission
scope, and warns that repository context and usage move to the alternate provider.
Only `yes` (case-insensitive, surrounding whitespace allowed) confirms. `no`,
blank input, EOF, unrecognized input and non-terminal input decline. There is no
unattended `--yes` override. The whole-run deadline and Ctrl+C also cover the wait.

| Failed role | Primary | Confirmed alternate | Scope |
| --- | --- | --- | --- |
| Implementation or repair | OpenCode | Codex | `workspace-write`, fresh ephemeral calls |
| Planning | Codex | OpenCode | Read-only tools, fresh session |
| Review | Codex | OpenCode | Read-only tools, fresh independent session |

Confirmation applies to that role for this run, including later repair calls
when implementation switches. Other roles keep their defaults and require their
own consent if they fail. Each role switches at most once; an alternate billing
failure stops without bouncing back. Launch, retry and repair limits still apply.
Partial changes are inspected before the question and evidence must remain
unchanged while answering. Protected files and the original baseline remain
protected. Cross-provider sessions are never resumed, and passing validation and
review remain mandatory for approval.

Configure the independent alternate settings under `fallback.codex_implementer`,
`fallback.opencode_planner`, and `fallback.opencode_reviewer`. Each has executable,
model, timeout and extra-argument settings; Codex adds reasoning/sandbox, OpenCode
adds variant/permission_policy. Flags use, for example,
`--fallback-codex-implementer-model` or `--fallback-opencode-reviewer-model`, with
matching `MULTIHARNESS_*` variables. Empty OpenCode models use its own default,
which the question explicitly labels `CLI default`; choose an explicit model for
predictable commercial deployments. No authentication or account is changed.

`--fallback-mode disabled` (`MULTIHARNESS_FALLBACK_MODE=disabled`) always stops
instead of asking. With JSON progress enabled, consent questions are deliberately
human-readable on stderr; stdout still contains only the final JSON. Disable
fallback for unattended consumers that require stderr to remain pure JSONL.
Separate concurrent CLI processes must not share one interactive input terminal.

## Colours and live progress

In a supported terminal, text mode now shows coloured stage labels, a single live
status line, elapsed time, the age of the latest agent update, repair rounds, retry
countdowns and a final evidence summary. Labels remain readable without colour:
`RUN`, `OK`, `INFO`, `WAIT`, `WARN`, `FAIL`, and `STOP`. Validation/review completion
is informational, not approval. The final summary reports the latest validation
and review evidence; it does not imply earlier failed repair rounds passed.

```sh
# Default: colours and animation when stderr is a suitable terminal.
./bin/multiharness --implementer-model opencode/big-pickle \
  --workdir /absolute/path/to/target-repository --task "Your task"

# Readable scrolling lines, without animation or colour.
./bin/multiharness --progress plain --color never \
  --workdir /absolute/path/to/target-repository --task "Your task"
```

| Setting | Values | Default |
| --- | --- | --- |
| `--color` / `MULTIHARNESS_COLOR` / `color` | `auto`, `always`, `never` | `auto` |
| `--progress` / `MULTIHARNESS_PROGRESS` / `progress` | `auto`, `plain`, `off` | `auto` |

Redirected stderr keeps the existing non-animated text logs by default. `plain`
selects readable scrolling lines even when redirected. `always` explicitly forces
colour for text output, including redirected output, but nonempty `NO_COLOR` and
`TERM=dumb` still disable colour. `NO_COLOR` disables only colour; choose `plain`
to disable motion too. Animation is disabled for redirected output, `TERM=dumb`,
nonempty `CI`, plain/off progress and JSON logging. Auto colour is disabled in CI.
`off` and `--quiet` suppress progress, not required billing-consent questions.

Agent activity comes from the existing Codex/OpenCode JSONL streams, without extra
model calls, polling requests, WebSockets or new UI dependencies. Codex metadata
follows the [official non-interactive event format](https://learn.chatgpt.com/docs/non-interactive-mode).
Only fixed activity labels are displayed; no commands, paths, messages, reasoning,
session IDs or provider diagnostics are copied to progress. An elapsed timer means
the stage is still open, not that the agent is making progress. Unknown/malformed
telemetry is ignored for display; existing failure and final-response parsers
remain authoritative. A provider's step-finish event is never approval.

The display coalesces activity in a one-slot buffer, refreshing at most four times
a second. Provider output readers never wait for terminal rendering. Activity is
best-effort, not a complete audit stream; fast intermediate updates may be omitted.
Stage transitions flush the latest activity before advancing. The live line adapts
to terminal width and never enables raw mode, hides the cursor or clears the screen.
Billing consent pauses rendering and resumes after the prompt ends, including
refusal, EOF and cancellation. Ctrl+C stops the renderer and cancels the workflow;
short/failed terminal writes also cancel and prevent a successful result.

This renderer belongs to the actual `multiharness` CLI. The text-only calculator
smoke test still uses `go test` logging and does not open the interactive CLI view.

## Output and exit codes

Normal invocations emit exactly one version-1 result JSON document on stdout,
including input/configuration failures. The envelope adds `schema_version`,
`task_id`, and `run_id` alongside the `TaskOutput` fields. Each invocation gets new,
random IDs; every repair in that invocation shares them. They are not session IDs
and are not derived from task content. Ordered lifecycle progress goes to stderr;
`--quiet` suppresses it. `--help` is the exception and writes usage text to stdout.
Task summaries, validation logs, diffs, and answers are in the JSON result rather
than mixed into progress lines. Treat saved results as potentially sensitive.

`log_format` / `MULTIHARNESS_LOG_FORMAT` / `--log-format` selects `text` (default)
or `json` (one version-1 JSON object per line). JSON logs contain timestamps,
task/run IDs, known event/stage/status/error codes and counters. Optional
`code=agent_activity` notices contain allowlisted `agent` and `activity` values,
with sequence zero; they do not change workflow event ordering. Retry lifecycle
events include `retry_delay_millis`. JSON never contains colours or animation,
even with `--color always`. No raw errors,
prompts, agent output, environment values, paths or diffs are logged; unexpected
string metadata is replaced with `[redacted]`. Full result evidence is not
redacted. See [security.md](security.md) for that distinction. Before configuration
has been validated, startup-error progress uses the safe text default.

The `result_ready` log precedes writing stdout, so a broken stdout can still make
the process exit unsuccessfully. When stderr remains available, a subsequent
`result_output_failed` notice (or a red `FAIL` line) explicitly reports that
delivery failure and exit 1. Consume the complete result and exit code, not a
progress log alone. A failed/short log write cancels execution and prevents success.

| Code | Meaning |
| --- | --- |
| 0 | `approved` coding result or `answered` non-coding response |
| 1 | Workflow, internal, or output failure |
| 2 | Usage, configuration, input-reading, or initialization failure |
| 3 | `repair_limit_reached`, without approval |
| 130 | `cancelled`, including deadlines |

Use the compiled binary when consuming exit codes: `go run` may itself return a
different exit code when the launched program exits unsuccessfully.

```sh
./bin/multiharness --config examples/multiharness.json \
  --workdir /absolute/path/to/target-repository --task-file task.txt \
  > result.json 2> progress.log
```

Write redirected results outside the target checkout, or to ignored paths, so
the CLI's own output files do not become concurrent repository changes.
