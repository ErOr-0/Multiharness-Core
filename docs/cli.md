# Local CLI

The CLI runs the plain-Go workflow service with configurable Codex/OpenCode
planning, Codex review, configurable Codex/OpenCode implementation/repair, Git evidence, and
configured deterministic checks. Planning defaults to Codex.
The CLI and repair loop are covered by deterministic tests with fake agents.
Opt-in authenticated-agent tests and their current verification status are
documented in [testing.md](testing.md).

## Build and run

Use the Go toolchain specified in `go.mod`. Install and authenticate Codex and
the other agent CLIs you select, and have Git available on `PATH`. Missing default agent
commands can offer an explicitly confirmed installation; see [setup.md](setup.md).
The CLI does not choose credentials or retry authentication automatically.

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

## Interactive magent

For Docker, create the named container once using [the Docker guide](docker.md),
then run `docker start -ai multiharness` from any directory. Source developers
can run `make build-dev` and use `dist/multiharness-dev`; this does not install
a host command or create a second Docker launch path.

Type a single-line task and press Enter to run it. Results are readable text,
with existing live progress on stderr. Ctrl+C cancels active work and exits;
`/quit` or Ctrl+D exits at the prompt. Completed tasks return to the prompt.
Each submission starts an independent workflow, without implicit chat history.

- `/config` offers numbered project/team choices inside Docker; the team form
  walks through planner and implementer selection and active agent models.
  Completed team configuration saves automatically; Enter
  keeps a value. Invalid answers retry only that field, retaining earlier answers.
  `/cancel` discards the entire unfinished setup.
- `/login codex` or `/login opencode` runs account setup in the same Docker container.
- `/workspace` selects and remembers a folder inside the mounted tree.
- `/settings` shows the repository, selected roles, check count and repair limit.
- `/set OPTION VALUE` changes any existing CLI configuration option without `--`.
  For example, `/set implementer-model provider/model` or `/set workdir /path/to/repo`.
  Values are literal strings or JSON according to the option; no shell is invoked.
- `/options` lists all settings, including reasoning, timeouts, permissions and checks.
- `/load PATH` loads an explicit version-1 configuration and clears session overrides.
- `/save` writes personal defaults outside the repository with private file permissions.
- `/help` lists commands.

Personal defaults live at `os.UserConfigDir()/magent/config.json` (on macOS,
`~/Library/Application Support/magent/config.json`; on Linux, usually
`~/.config/magent/config.json`). They load only in interactive mode. An explicit
`MULTIHARNESS_CONFIG` replaces that file; environment values override loaded
settings, and `/set` overrides the environment. `/save` saves effective settings
and clears the implementation session ID. Team defaults remain portable; Docker
workspace selection is stored separately in `workspace.json` beside the settings
file and revalidated against the current mount on every start. Invalid settings are rejected before replacing the
current configuration. Authentication remains with the provider CLIs.

The interactive screen uses a cyan heading and prompt, aligned agent roles,
green success labels and amber correction messages. Labels remain meaningful
without colour. It shares the existing `color` preference with task progress:
`NO_COLOR`, `TERM=dumb` and `/set color never` disable colour. The prompt divider
adapts to the terminal width.

Interactive configuration accepts command/option names in either case, whitespace
or `=` separators, optional `--` on option names, underscores in place of hyphens,
complete outer quotes (including pasted smart quotes), and leading zeros in numeric
settings. For example, `/SET --planner_harness = CODEX` selects Codex.
Unknown command and option names get a nearby spelling suggestion when unambiguous;
the suggestion never executes automatically. Missing values retain current settings;
use an explicit `""` to clear an optional value. Malformed quotes, invalid UTF-8,
control bytes and malformed settings get actionable errors.

Model IDs retain their exact spelling and case after surrounding quotes/whitespace
are removed. Model syntax is validated locally; provider availability and account
access are checked on actual use. Model IDs, paths, JSON contents, permission tokens,
and task prose are never fuzzily corrected or shell-expanded. This is intentionally
interactive-only; scripts and JSON configuration retain their existing strict
contracts. The recovery approach follows the
[Command Line Interface Guidelines](https://clig.dev/#help): offer a correction
without silently executing a guessed action.

Validation defaults remain empty. To configure Go tests in the prompt:

```text
/set validation-checks [{"executable":"go","args":["test","./..."]}]
/save
```

Explicit task arguments keep the existing JSON interface, for example
`magent --task "Explain this repository"`. Redirected/CI invocations never open
the interactive prompt. Opening or configuring the prompt makes no model calls.

## Codex implementation without OpenCode

Choose each role's CLI independently from its model. For example, in the prompt:

```text
/set planner-harness codex
/set planner-model gpt-6-astra
/set implementer-harness codex
/set implementer-model gpt-5.6-luna
/set fallback-mode disabled
/save
```

This uses Codex's saved login for Astra planning, Luna implementation/repair and
the configured Codex reviewer (Sol by default). OpenCode login is unnecessary for
this configuration. Select models available to your account; the app does not
grant model access. `/config` offers the same choices.

For scripted runs, see [`examples/codex-team.json`](../examples/codex-team.json),
or use `--implementer-harness codex --implementer-model gpt-5.6-luna`. Set your
project's validation commands explicitly; the example does not invent checks.
Codex implementation uses fresh workspace-write invocations for each attempt,
with full task/plan/review evidence supplied again. Planning/review stay read-only.
OpenCode implementation retains its same-session repair behavior.

Existing version-1 files without `implementer.harness` keep OpenCode as the
implementer. The existing implementation billing fallback is OpenCode to Codex;
a primary Codex implementer stops on billing failure instead of switching to
itself. Fallback remains independently configurable for planning/review.

## Planning harness and simple answers

`--planner-harness codex` is the default. Codex uses `--planner-model`
(`gpt-5.6-sol` by default) and `--planner-reasoning` (`xhigh` by default).
To select OpenCode for planning and simple answers:

```sh
./bin/multiharness --workdir /absolute/path/to/target-repository \
  --planner-harness opencode --planner-model provider/model \
  --task "Explain what this repository does."
```

The selected planner makes the same explicit `answer` or `implement` decision.
An answer ends the run immediately after repository checks. A coding plan
continues to OpenCode implementation and independent Codex review. There is no
second classifier call or model-selected harness routing.

The version-1 JSON uses one planner object for either provider:

```json
{
  "version": 1,
  "planner": {
    "harness": "opencode",
    "model": "provider/model"
  }
}
```

`planner` contains `harness`, `executable`, `model`, `timeout`, `extra_args`,
`reasoning` (Codex), `variant` (OpenCode), `sandbox` and `permission_policy`.
Planning remains read-only for both providers. All flags use `--planner-*` and
all environment variables use `MULTIHARNESS_PLANNER_*`. The selected harness
controls default executable/model/reasoning values; explicit values and pins
retain normal file → environment → flag precedence. Omitted harness selects
Codex. An omitted OpenCode model uses its CLI default.

The optional `fallback.planner` uses the same structure and defaults to the other
harness. Its flags use `--fallback-planner-*`. A recognized billing failure still
requires explicit terminal consent before this alternate can run. The alternate
must differ from the primary when fallback is enabled. Refusal, non-interactive
input and `--fallback-mode disabled` stop without switching.

In `/config`, changing harness resets provider-specific planner and fallback
settings (model, executable, reasoning, variant and extra arguments) to matching
defaults, while keeping timeouts. This prevents old provider options from leaking
into the newly selected CLI. Set custom pins/options again after switching, then
`/save`. Scripted configuration never silently rewrites explicit pins or models.

This pre-release replaces `planner_harness`, `opencode_planner`, and
`fallback.opencode_planner`; those old properties and `--opencode-planner-*` flags
are rejected. Move the selected provider settings into `planner`, add its `harness`,
and move the alternate settings into `fallback.planner`. Old Codex configurations
that only contain `planner` still work unchanged. See [versioning](versioning.md).

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
file must declare `"version": 1`. Configuration properties use their documented
lowercase spelling; keys inside `env_overrides` retain their original case.
Unknown properties, duplicate keys (including case-only variants), invalid UTF-8, null values,
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
an answer-only task requires only the selected planning harness to be installed.

## Execution and permissions

The planner emits a version-2 decision:

- `implement`: a plan with steps and acceptance criteria, followed by
  implementation → validation → review → repair when needed.
- `answer`: a direct response, with no implementation, deterministic checks,
  or separate review. An answer can ask for clarification; it is not approval of
  a code change. An ambiguous or malformed decision fails closed.

Both paths accept an accessible workspace folder: a project, a subfolder, or a
parent containing multiple projects. Git repositories are optional. The same
workspace safety checks apply; see [workspace behavior](workspaces.md).
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
| Planning | User-selected Codex or OpenCode | Other configured planner | Read-only tools, fresh session |
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
`fallback.planner`, and `fallback.opencode_reviewer`. Each has executable,
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
Both stdin and stderr must be terminals, and `CI` must be unset or empty, for a
question to appear. Redirected prompts cannot authorize a switch.

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

Within the workflow, malformed task input reports `invalid_input`. An inaccessible
or unsupported checkout, failed workspace lock, or failed baseline capture reports
`workspace_error` during intake; no agent starts in these cases.

Use the compiled binary when consuming exit codes: `go run` may itself return a
different exit code when the launched program exits unsuccessfully.

```sh
./bin/multiharness --config examples/multiharness.json \
  --workdir /absolute/path/to/target-repository --task-file task.txt \
  > result.json 2> progress.log
```

Write redirected results outside the target checkout, or to ignored paths, so
the CLI's own output files do not become concurrent repository changes.
