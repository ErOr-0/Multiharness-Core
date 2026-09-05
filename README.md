# Multiharness Core

A plain-Go coordinator for **Codex planning → OpenCode implementation →
deterministic checks → Codex review → OpenCode repairs**. A non-coding request
can instead receive a direct planner answer. Planning and simple answers use
Codex by default; `--planner-harness opencode` selects OpenCode.

Approval requires a valid reviewer decision, passing configured checks, and
independent Git evidence. Exhausting repairs is a failure to obtain approval,
not success. Approval means these gates passed; it does not promise perfect code.

## Getting started

Requirements: the Go toolchain in `go.mod` (currently 1.26.6), Git, and separately
installed/authenticated Codex and OpenCode CLIs. Workspace locking supports
macOS, Linux, FreeBSD, OpenBSD, NetBSD, and DragonFly BSD. Windows is not supported.

Use the official [Codex CLI](https://developers.openai.com/codex/cli/) and
[OpenCode CLI](https://opencode.ai/docs/cli/) installation instructions.
Codex supports an existing ChatGPT login; this project does not itself require
an OpenAI API key or manage credentials. See [Codex authentication](https://developers.openai.com/codex/auth/).
OpenCode needs an authenticated provider and an available model.

Missing default agent CLIs can offer an explicit installation prompt on macOS/Linux
when npm is available. CI/piped runs never install. Use `--install-mode disabled`
for managed environments. Installation stops the current run for sign-in and a
fresh invocation; it never replays work or claims approval. Missing Git/npm and
explicit executable pins receive manual setup guidance. See
[dependency setup and failure cases](docs/setup.md).

With the default `codex` executable, Multiharness automatically selects a
compatible installed CLI when a newer app has written an incompatible model
cache. On macOS it also checks the Codex/ChatGPT app bundles. No manual PATH
workaround, shared-cache deletion, global upgrade, or model change is needed when
a compatible installation exists. See [runtime recovery](docs/cli.md#automatic-codex-runtime-recovery).

```sh
go build -o ./bin/multiharness ./cmd/multiharness
./bin/multiharness --help
./bin/multiharness --config examples/multiharness.json \
  --workdir /absolute/path/to/target-repository \
  --task "Implement the requested change and add regression tests."
```

Start with a disposable repository. Edit the example's OpenCode `model` and
`variant` for your provider; an empty value uses the CLI's own default. Planning
and review default to `gpt-5.6-sol`, `xhigh`, read-only. Agent executables, models,
reasoning, variants, permissions, timeouts, checks, and repair limits are
configurable. Precedence: defaults < explicit JSON file < environment < flags.

For OpenCode planning or simple answers, use `--planner-harness opencode` with
`--opencode-planner-model provider/model` and optional
`--opencode-planner-variant`. Codex settings stay under `--planner-*`;
implementation and review keep their configured roles. See
[planning harness selection](docs/cli.md#planning-harness-and-simple-answers).

The example runs Go tests and vet. Replace those checks for other projects.
Built-in defaults have **no validation commands**; an empty successful report
does not mean tests ran. Auto-approval of OpenCode permissions is off by default.

Use `--task-file` to avoid task text in process arguments and shell history.
Normal runs return one versioned JSON result on stdout, with correlated lifecycle
logs on stderr. `--log-format json` selects JSONL logs; `--quiet` suppresses them.
On a terminal, text output includes colour-coded stages, live activity and elapsed
time. Use `--progress plain --color never` for readable scrolling output, or
`--progress off` to suppress it. `NO_COLOR` is respected; JSON output never gets
terminal escapes. See [live progress](docs/cli.md#colours-and-live-progress).
Keep saved results outside the target checkout, and treat their contents as
sensitive. Only lifecycle logs are redacted; repository evidence is not altered.

## Outcomes

| Status | Exit | Meaning |
| --- | --- | --- |
| `approved` | 0 | Coding checks, repository safeguards, and review passed |
| `answered` | 0 | Planner answered without coding or an independent review |
| `failed` | 1 or 2 | Runtime failure (1), or usage/configuration/startup error (2) |
| `repair_limit_reached` | 3 | Blocking findings remain; no approval |
| `cancelled` | 130 | Cancellation or deadline; changes may remain |

## Boundaries and limitations

- Target a supported Git repository root. Non-Git/bare repositories, submodules,
  nested repositories, unresolved merges, and sparse checkouts fail explicitly.
- Existing dirty files are protected as whole files; agents cannot safely edit
  those same files in this version. Ignored artifacts are outside Git evidence.
- The cooperative Git lock is not an operating-system sandbox. Review agent
  permissions and provider settings before use. There is no automatic rollback,
  commit, push, or recovery/resume of interrupted workflows. Provider retries are
  off by default and can only replay read-only planning/review, never mutations.
- Billing/quota and access failures stop with safe structured diagnostics. Agent
  launches are bounded (64 by default, including retries); this is **not a dollar
  cap**. See [provider failure policy](docs/provider-failures.md) before commercial use.
- On exhausted billing, an interactive CLI can ask to switch the failed role:
  OpenCode implementation/repair → Codex, or Codex planning/review → OpenCode.
  An explicitly selected OpenCode planner can instead switch to Codex.
  Only an explicit `yes` authorizes the switch; piped input, EOF, blank input,
  and refusal never do. Use `--fallback-mode disabled` to suppress this option.
- Review is independent from the implementation session, but all agents
  and configured validation commands still need an appropriate trust boundary.
- Live compatibility depends on CLI versions, authentication, available models,
  and local agent configuration. Phase 9's authenticated smoke gate remains
  pending until the tests in [testing.md](docs/testing.md) pass.

## Development and documentation

For one real Codex → OpenCode → Codex test that prints calculator code without
saving or running the app, see the [text-only calculator demo](docs/testing.md#one-simple-live-calculator-test).

```sh
make check
make coverage
make security lint-workflows
```

Ordinary Go tests focus on planning, implementation, review/repair, context
handoff and provider failures. `make integration` runs the workflow and provider
scenarios through real adapters with fixture subprocesses. Opt-in tests create disposable repositories
and consume the caller's configured provider usage. `make check` forces live tests
off and runs formatting, module, unit/integration, race, vet, build and fuzz checks.
The security and workflow-lint targets fetch pinned tools and need network access.
GitHub CI is configured for Linux and macOS; it receives no provider credentials.

- [CLI and configuration](docs/cli.md)
- [Architecture and code reading order](docs/architecture.md)
- [Coverage measurement and remaining gaps](docs/coverage.md)
- [Dependency installation and startup failures](docs/setup.md)
- [Trust, permissions, and sensitive data](docs/security.md)
- [Interrupted-run recovery](docs/recovery.md)
- [Billing, provider failures, and execution limits](docs/provider-failures.md)
- [Tests and live compatibility checks](docs/testing.md)
- [Schema and release versioning](docs/versioning.md)
- [Outstanding release gates](docs/release-readiness.md)
- [Implementation checklist](AGENTS.md)

The application is a modular monolith with one synchronous workflow service.
The workflow owns policy and small ports; `internal/store` owns serializable
contracts and invariants. Process, Git, validation, and agent adapters are outer
dependencies. CLI presentation/configuration stay outside the core.
`cmd/multiharness` is the composition root. No persistence service or workflow
framework is needed. Start with `internal/workflow/service.go` for the complete
sequence and repair loop, then read the relevant role's stage function.
