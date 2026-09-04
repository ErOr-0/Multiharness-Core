# Multiharness Core Implementation Checklist

## Project goal

Multiharness accepts a user task and coordinates a configurable workflow:

1. A planning agent either answers a non-coding request directly or creates a structured implementation plan.
2. An implementation agent executes the plan in the target repository.
3. A review agent inspects the result and validation evidence.
4. Blocking review findings are returned to the implementation agent for repair.
5. Coding workflows stop only when the result is approved or a clearly reported terminal condition is reached. Non-coding responses use the distinct `answered` terminal status and never invoke the implementation agent.

Default target configuration:

- Planner: Codex with `gpt-5.6-sol` and `xhigh` reasoning.
- Implementer: OpenCode with a configurable model and variant.
- Reviewer: Codex with `gpt-5.6-sol` and `xhigh` reasoning.

All commands, models, reasoning levels, variants, timeouts, and retry limits must remain configurable.

Runtime architecture:

- Use the plain-Go `workflow.Service` as the application entry point. Construct it with `workflow.NewService` and call `Service.Run(ctx, input)` for a typed `TaskInput` → `TaskOutput` workflow.
- Execute stages as ordinary Go methods, retain explicit contract validation, and propagate the caller's context through every agent handoff.
- Publish ordered stage and terminal events through the workflow-owned `EventSink`. Richer tracing is optional future adapter work, not a prerequisite for running a task.
- Keep Codex and OpenCode behind workflow-owned ports; domain contracts must not depend on a model provider, CLI, or orchestration framework.
- Keep the runtime deterministic: the planner selects only the explicit `implement` or `answer` branch. No conversational-session engine, open-ended model-directed tool routing, or persistent workflow engine is required for the current use case.

## Working rules

- Work through the phases below in order unless a prerequisite requires a small adjustment.
- Keep the workflow package independent of Codex, OpenCode, and operating-system details.
- Construct the workflow service at the composition root and use the same `Service.Run` entry point for production and integration tests.
- Add or update tests with each behavior change.
- Do not mark an item complete until its tests and relevant static checks pass.
- Treat the architecture and engineering principles below as part of every phase completion gate.
- After completing a phase, update this checklist and stop for a progress review before starting the next phase.
- Preserve unrelated user changes in target repositories.
- Never report success merely because a retry limit was reached.

## Architecture and engineering principles

Apply SOLID in a Go-appropriate, pragmatic way:

- **Single responsibility:** Keep each package, type, and function focused on one reason to change. Separate workflow policy, process execution, agent adapters, configuration, repository inspection, and presentation.
- **Open/closed:** Add new agent providers and execution strategies through stable interfaces and composition instead of changing workflow policy for every provider.
- **Liskov substitution:** Every implementation of a port must honor the same inputs, outputs, cancellation behavior, error semantics, and side-effect boundaries.
- **Interface segregation:** Keep interfaces small and consumer-owned. Do not force components to depend on methods they do not use.
- **Dependency inversion:** Core workflow policy depends on abstractions. Codex, OpenCode, Git, the operating system, configuration sources, and CLI/API delivery remain outer adapters injected at composition boundaries.

Also enforce these engineering rules:

- Maintain clear dependency direction toward the workflow domain; avoid import cycles and hidden global dependencies.
- Prefer explicit constructor injection and immutable validated configuration over package-level mutable state.
- Keep domain contracts independent of vendor SDK and CLI response types.
- Prefer composition, cohesive packages, and straightforward control flow.
- Apply KISS and YAGNI: introduce abstractions for an existing boundary or tested variation point, not a hypothetical one.
- Apply DRY to shared knowledge and invariants, but do not merge code that only looks similar while serving different responsibilities.
- Make side effects, permissions, retries, timeouts, and terminal conditions explicit.
- Return contextual, inspectable errors and preserve cancellation semantics.
- Keep behavior deterministic where possible and test through public contracts rather than implementation details.
- Review naming, cohesion, coupling, dependency direction, failure handling, security boundaries, observability, and testability before completing each phase.
- Document any deliberate deviation or architectural tradeoff in the relevant code or checklist before accepting it.

## Target package architecture

Use a feature-oriented workflow core with ports-and-adapters boundaries:

```text
cmd/multiharness/
└── main.go                       # composition root

internal/store/
├── task.go                       # task input and repair-limit semantics
├── plan.go                       # planning contract and invariants
├── implementation.go             # implementation request/result contracts
├── validation.go                 # validation request/report contracts
├── review.go                     # review and repair contracts
├── repository.go                 # independent checkout evidence and invariants
├── result.go                     # terminal statuses, failures, and output
├── provider_failure.go           # safe provider-neutral failure categories
└── errors.go                     # shared contract-validation error

internal/workflow/
├── service.go                    # high-level orchestration sequence
├── service_config.go             # dependency validation and construction
├── run_state.go                  # private state for one workflow run
├── intake_stage.go               # input and workspace readiness
├── planning_stage.go             # structured planning execution
├── implementation_stage.go       # initial implementation execution
├── validation_stage.go           # deterministic validation execution
├── review_stage.go               # evidence review and decision checks
├── repair_stage.go               # rejected-review repair execution
├── repository_evidence.go        # evidence integrity and preservation policy
├── agent_execution.go            # safe retries and whole-agent invocation limits
├── stage_failure.go              # stage-to-terminal failure context
├── outcome.go                    # terminal-result construction
├── output.go                     # stable JSON collection shape for results
├── events.go                     # structured lifecycle events
├── ports.go                      # consumer-owned dependency abstractions
└── doc.go                        # workflow state-machine policy

internal/adapter/
├── agent/
│   ├── codex/                    # planner and reviewer implementations
│   ├── opencode/                 # implementer implementation
│   └── provider/                 # bounded error-stream monitoring and translation
├── process/                      # operating-system command execution
├── validation/                   # configured deterministic command checks
├── workspace/git/                # repository state, diffs, and snapshots
└── persistence/                  # optional run persistence; add only when needed

internal/transport/
├── cli/                          # CLI handler
└── http/                         # optional future HTTP handler

internal/config/                  # configuration loading and validation
```

Dependency rules:

- CLI and HTTP handlers call `workflow.Service.Run`; handlers never contain workflow policy.
- The workflow service depends only on workflow-owned ports and focused contracts from `internal/store`.
- Concrete adapters import the workflow package to satisfy its ports; the workflow package never imports adapters.
- Contract validation and terminal-result construction live in the workflow and store packages. Transport adapters own decoding and encoding, with no framework runtime required.
- Keep task, plan, implementation, validation, review, and result contracts in their corresponding `internal/store` files; do not collect them into a generic model file.
- `internal/store` owns serializable workflow state and invariants; it is not a filesystem or database implementation.
- Add a workflow repository port only when persisted workflow state is required. Keep its database implementation in an outer adapter.
- Treat Git checkout inspection as a workspace/change-tracking adapter, not as a domain repository.
- The composition root constructs `workflow.Service` with concrete workspace, planner, implementer, validator, reviewer, and optional event-sink adapters.

## Phase 1 — Foundation

- [x] Initialize the Go module and modular internal package structure.
- [x] Define a context-aware process execution contract; keep runner interfaces consumer-owned at integration points.
- [x] Support command arguments, working directory, timeout, output capture, and exit codes.
- [x] Define initial workflow types for task input, plan, review, and task output.
- [x] Define planner, implementer, and reviewer ports.
- [x] Add process-runner success and non-zero-exit tests.
- [x] Verify the foundation with `go test ./...` and `go vet ./...`.

## Phase 2 — Workflow contract

- [x] Define and document the workflow state machine and stopping conditions.
- [x] Validate task text, working-directory input, and review/repair limits using pure contract validation.
- [x] Delegate working-directory availability and permissions to a workflow-owned workspace port.
- [x] Replace ambiguous completion state with explicit terminal statuses such as approved, failed, cancelled, and review-limit-reached.
- [x] Define an implementation result containing summary, changed files, and optional agent session ID.
- [x] Keep deterministic validation evidence in a separate validation report and validator port.
- [x] Define structured review findings with severity, blocking status, location, evidence, and required action.
- [x] Pass the original plan and latest implementation evidence into review and repair operations.
- [x] Define cohesive implementation, validation, review, and repair request contracts.
- [x] Split task, plan, implementation, validation, review, and result contracts into responsibility-focused `internal/store` files.
- [x] Clarify whether the configured limit counts reviews or repair attempts; prefer an explicit repair-attempt limit.
- [x] Add JSON serialization tests for every cross-agent contract.

### Phase 2 completion gate

- [x] Workflow contracts can represent approval, required repair, agent failure, cancellation, and exhausted repair attempts without relying on free-form text.
- [x] Workflow contracts and validation remain independent of filesystem, agent, transport, and persistence implementations.
- [x] Package responsibilities and dependency direction pass the architecture review.
- [x] `go test ./...` and `go vet ./...` pass.

## Phase 3 — Process execution hardening

- [x] Add stdin support for prompts and structured payloads.
- [x] Add controlled environment-variable overrides without discarding the inherited environment.
- [x] Support streamed stdout/stderr or structured progress events.
- [x] Bound retained command output while preserving useful failure diagnostics.
- [x] Distinguish timeout, cancellation, missing executable, invalid working directory, and non-zero exit failures.
- [x] Ensure cancellation terminates spawned process trees where supported.
- [x] Move operating-system execution into the focused `internal/adapter/process` package and keep consumer interfaces outside it.
- [x] Add tests for stdin, working directory, timeout, cancellation, missing executable, environment overrides, and output limits.

### Phase 3 completion gate

- [x] Long-running agent commands can report progress and be cancelled without leaking child processes or unbounded output.
- [x] Process responsibilities, dependency direction, and consumer-owned interface policy pass the architecture review.
- [x] `go test ./...` and `go vet ./...` pass.

## Phase 4 — Orchestrator

- [x] Implement a dependency-injected orchestrator using the workflow ports.
- [x] Execute plan, initial implementation, validation, and review in the required order.
- [x] Return blocking findings to the implementer until approved or the repair limit is reached.
- [x] Preserve context cancellation across every stage.
- [x] Emit structured workflow events for stage start, progress, completion, and failure.
- [x] Expose a typed Go service entry point using the focused store input and output contracts.
- [x] Execute workflow stages directly with the caller's context and ordered lifecycle events.
- [x] Produce a final task result from recorded evidence rather than agent claims alone.
- [x] Add fake-agent tests for immediate approval, repair then approval, repeated rejection, planner failure, implementer failure, reviewer failure, and cancellation.
- [x] Add service-entry, context-propagation, output-serialization, dependency-validation, and cancellation tests.

### Phase 4 completion gate

- [x] The complete state machine is covered by deterministic unit tests without invoking external agent CLIs.
- [x] The plain-Go service owns typed workflow execution and lifecycle events without depending on a framework runtime.
- [x] `go test ./...` and `go vet ./...` pass.

## Phase 5 — Codex planner and reviewer

- [x] Add separate Codex adapter modes for planning and reviewing.
- [x] Make executable path, model, reasoning effort, timeout, sandbox, and extra arguments configurable.
- [x] Default planning and review to `gpt-5.6-sol` with `xhigh` reasoning.
- [x] Run planning and review with read-only repository access.
- [x] Define versioned JSON schemas for plan and review output.
- [x] Parse and validate schema-constrained final responses.
- [x] Keep reviewer execution independent from the implementation session.
- [x] Include task, plan, repository state, diff, and validation evidence in the review contract.
- [x] Add argument-building, output-parsing, malformed-output, and command-failure tests.

Repository evidence boundary for this phase: every fresh reviewer invocation is
required to inspect the live status, tracked diff, and untracked files itself and
to treat implementation summaries as untrusted claims. Phase 7 will add captured
pre-implementation baselines and workflow-owned change attribution; that
trustworthy historical evidence cannot be reconstructed by the Phase 5 adapter.

### Phase 5 completion gate

- [x] Planner and reviewer adapters satisfy the workflow ports and reject invalid structured output.
- [x] `go test ./...` and `go vet ./...` pass.

## Phase 6 — OpenCode implementer

- [x] Add an OpenCode adapter for initial implementation and review repairs.
- [x] Make executable path, provider/model, variant, timeout, permission policy, and extra arguments configurable.
- [x] Capture the OpenCode session ID from the initial implementation.
- [x] Resume the same session for repair rounds when possible.
- [x] Supply the original task, approved plan, current validation evidence, and structured blocking findings to repair runs.
- [x] Parse JSON event output and surface useful progress events.
- [x] Define safe behavior when interactive permission input would otherwise block execution.
- [x] Add argument-building, event-parsing, session-resume, malformed-output, and command-failure tests.

OpenCode adapter boundary for this phase: prompts are supplied on stdin to keep
task content out of process arguments, while stdout is parsed as bounded JSONL.
The adapter requires a valid, versioned implementation result in the last text
event and independently captures a consistent session ID. A step-finish event is
treated as progress rather than completion evidence so a missing terminal event
cannot turn incomplete output into success. Agent error events, malformed output,
missing final text, and session mismatches fail explicitly. By default, OpenCode
keeps its non-interactive reject-on-prompt behavior; `--auto` is available only
through the explicit auto-approve policy, and configured deny rules still apply.
Agent-reported changed files remain untrusted until Phase 7 derives changes from
repository evidence.

### Phase 6 completion gate

- [x] OpenCode can implement a plan and apply a later structured review within the same task context.
- [x] `go test ./...` and `go vet ./...` pass.

## Phase 7 — Repository evidence and validation

- [x] Capture repository state before implementation begins.
- [x] Derive changed files and diffs independently of agent-reported output.
- [x] Preserve pre-existing user changes and identify them separately from workflow changes.
- [x] Add configurable deterministic validation commands.
- [x] Record command, exit code, duration, and bounded output for each validation step.
- [x] Prevent concurrent workflows from mutating the same working directory.
- [x] Define behavior for non-Git working directories.
- [x] Add tests using temporary Git repositories.

Phase 7 boundaries and deliberate tradeoffs:

- Intake acquires a workspace lease and captures a stable baseline before any
  agent runs. Every implementation and repair is compared with that original
  baseline. Observed changed files replace agent claims. Planning, validation,
  and review must leave the inspected checkout unchanged; incomplete or stale
  evidence cannot result in approval.
- Pre-existing dirty paths, including untracked files and deletions, are
  protected at whole-file granularity. Tasks that need to edit those paths must
  start after the user resolves the existing changes. Changes to protected
  paths, index entries, or HEAD stop the run. Preservation violations and
  inspection failures retain a private recovery directory with baseline files,
  absent-file records, and an index-entry manifest. No automatic rollback,
  staging, stashing, or committing occurs. Normal-run baselines are in memory;
  crash-resilient persistence is not implemented in this phase.
- Evidence covers tracked and non-ignored untracked files, contents, symlink
  targets, and modes. Ignored artifacts are outside the boundary. Diffs retain
  Git's relative `before/` and `after/` comparison-tree prefixes without rewriting
  filenames; the changed-file list uses repository-relative paths. Snapshot and
  diff limits fail closed, rather than approving truncated repository evidence.
- Non-Git/bare directories, subdirectory targets, nested repositories,
  submodules, unmerged indexes, sparse/skip-worktree and assume-unchanged entries,
  special files, and non-UTF-8 paths fail explicitly. Cooperative OS locks cover
  the Git common directory, including linked worktrees, on supported Unix
  systems. Other platforms fail closed. Locks and post-stage checks are not a
  sandbox against arbitrary commands or concurrent human edits.
- Validation executes configurable argv commands with per-command timeouts,
  environment overrides, and bounded output. Ordinary check failures become
  review evidence; infrastructure errors and cancellation stop execution while
  retaining completed check results. An empty check list explicitly means no
  configured checks ran, not proof that tests passed. Configuration loading and
  CLI wiring are provided in Phase 8.
- A `workflow.Service.Run` integration test uses a real temporary Git repository
  and real validation commands with fake agents. It covers rejection, repair,
  approval, original-baseline retention, and preservation of unrelated user
  notes. Real Codex/OpenCode smoke tests remain Phase 9 work.

### Phase 7 completion gate

- [x] Final and review results contain trustworthy repository and validation evidence.
- [x] Workspace ownership, evidence policy, validation execution, dependency direction, and documented security boundaries pass the architecture review.
- [x] `go test ./...` and `go vet ./...` pass.

## Phase 8 — Configuration and CLI vertical slice

- [x] Define configuration precedence for defaults, config file, environment variables, and CLI flags.
- [x] Validate configured executables, models, reasoning levels, variants, durations, and limits.
- [x] Add a CLI entry point that accepts a task and target working directory.
- [x] Stream concise workflow progress to stderr and emit the final structured result to stdout.
- [x] Return meaningful process exit codes for approved, answered, failed, cancelled, and review-limit outcomes.
- [x] Document a local example configuration for Codex planning/review and OpenCode implementation.
- [x] Add CLI tests with fake adapters.
- [x] Add an explicit, validated answer-only planning branch that skips implementation, validation, review, and repair while retaining workspace safety checks.
- [x] Exercise the production composition root with real Git/process/validation adapters and fixture executables speaking both agent protocols.

Phase 8 boundaries and deliberate tradeoffs:

- `cmd/multiharness` is the composition root; the CLI calls the plain-Go workflow
  service. Configuration and presentation remain outside workflow/store. The
  environment-option mapping is shared with flag registration to prevent drift.
  No new external module dependencies were added.
- Configuration precedence is defaults → explicitly selected version-1 JSON
  file → environment → explicit flags. No target-repository config is discovered
  automatically. Unknown properties, duplicate/case-variant keys, null values,
  invalid UTF-8, invalid durations/limits, and unsupported versions fail before
  any agent starts. Zero repairs and empty optional strings/collections are real
  overrides, not defaults. Runtime command retries were disabled in this phase;
  Phase 9 adds explicit, bounded read-only provider retries with a zero default.
- Application paths are relative to the invocation directory; explicit relative
  validation executable paths are relative to the target repository. Executable
  syntax is validated during configuration; availability and authentication are
  checked when that command is invoked. Answer-only tasks therefore do not need
  installed OpenCode or reviewer executables. Agent/model availability is not
  guessed or probed through model calls at startup.
- Codex planning/review are restricted to read-only through CLI configuration.
  Managed extra-argument overrides are rejected, including attached short Codex
  flags and the `--` delimiter. OpenCode auto-approval remains an explicit opt-in.
- The planner wire schema is now version 2 and requires an explicit `implement`
  or `answer` decision. Version 1 planner output is rejected; review and OpenCode
  implementation wire schemas remain at version 1. Domain plan contracts require
  an explicit action. An answer-only plan needs a nonblank answer and no coding
  steps/acceptance criteria; coding ports reject answer-only plans. The distinct
  `answered` result cannot contain implementation, validation, review, repair, or
  changed-workspace evidence. Answers can request clarification; they are not
  coding approvals. Both branches still require a supported Git repository root.
- The CLI accepts one quoted positional task, `--task`, or a bounded regular
  UTF-8 `--task-file`. Stdin is deliberately not supported in this phase. Task
  files avoid placing task content in shell history/process arguments. Progress
  prints controlled event metadata, not prompts or raw provider output. Full
  JSON results can still contain sensitive repository/validation data; secret
  redaction and correlated logging remain Phase 9 work.
- The default deterministic-check list is empty. The CLI warns when validation
  runs without checks unless quiet mode is enabled. `examples/multiharness.json`
  supplies Go test/vet checks; callers must choose checks appropriate to their
  target project. The CLI does not invent test commands or claim they ran.
- Exit codes are 0 for approved/answered, 1 for workflow/internal/output failure,
  2 for usage/config/input/initialization failure, 3 for exhausted repairs, and
  130 for cancellation/deadlines. Normal invocations emit one JSON result;
  `--help` emits usage text. Whole-run and stage deadlines are bounded, and
  Ctrl+C/SIGTERM propagate cancellation through the existing process adapter.
- Tests cover CLI/config boundaries, output failures, timeout/cancellation,
  schema decisions, and the real composition root with fixture subprocesses.
  The fixture integration verifies a full repair cycle, limit exhaustion,
  answer-only execution with missing unused executables, missing-planner failure,
  independent change attribution, and preservation of pre-existing user notes.
  No authenticated model calls were made. See `docs/cli.md` for local usage.

### Phase 8 completion gate

- [x] One local command can run the complete plan → implement → validate → review → repair workflow.
- [x] Configuration, composition, delivery, agent protocols, and workflow policy retain clear responsibility and dependency boundaries.
- [x] `go test ./...` and `go vet ./...` pass.

## Phase 9 — Integration and release readiness

- [x] Add opt-in smoke tests for installed and authenticated Codex and OpenCode CLIs.
- [ ] Test immediate approval and at least one repair loop in a disposable repository.
- [ ] Verify timeout and cancellation behavior against both real CLIs.
- [x] Add structured logging with task/run correlation IDs and secret redaction.
- [x] Document trust boundaries and the permissions granted to implementation agents.
- [x] Document recovery steps for interrupted workflows.
- [x] Add a README covering installation, configuration, usage, statuses, and limitations.
- [x] Define versioning for configuration and cross-agent JSON schemas.
- [x] Normalize billing, rate-limit, overload, authentication, access and unknown provider errors without exposing raw diagnostics.
- [x] Stop reported provider failures promptly, including pre-session and exit-zero error events.
- [x] Add opt-in bounded read-only retries and per-run agent invocation limits; never replay implementation/repair automatically.
- [x] Add strict Cucumber/Gherkin workflow and provider-failure acceptance tests, focused unit tests, and malformed-error fuzz coverage.
- [x] Add explicit terminal consent for billing-only role fallback: OpenCode implementation/repair to Codex, and Codex planning/review to OpenCode.
- [x] Verify refusal/EOF/non-interactive handling, alternate failure, partial-work handoffs, role stickiness, and safe cross-provider session boundaries.
- [x] Add separately opted-in real-agent billing-handoff smoke scenarios for all four stages, with deterministic offline tests of the harness itself.
- [x] Reject ambiguous duplicate/nested/escaped JSON keys, noncanonical key casing and invalid UTF-8 agent responses; add unit and Gherkin regressions.
- [x] Add repeatable local release checks and pinned Linux/macOS CI configuration without provider credentials or live calls; remote CI execution remains pending.
- [x] Add production import-graph regression checks and vulnerability scanning; update the Go toolchain and x/sys to patched versions.
- [x] Add one opt-in text-only calculator demo: real Codex planning, OpenCode code generation, and independent Codex static review, with no app files saved or executed.
- [x] Recover default Codex CLI/cache version mismatches through bounded offline selection of a compatible installed runtime; preserve explicit pins, model/permissions, cancellation and single task execution.
- [x] Add terminal-aware colours, live safe agent activity, elapsed/retry timing, repair labels and evidence summaries; preserve JSON output, redaction, consent, cancellation and bounded buffering.

Phase 9 implementation and verification boundary:

- Opt-in tests are in `cmd/multiharness/smoke_test.go`. Normal tests skip live
  agents. Full smoke workflows reuse the production composition, require an
  explicitly selected OpenCode model, create disposable repositories, and check
  independent Git attribution, preserved user notes, deterministic Go tests and
  approval. The repair scenario injects a subtraction fault after the first real
  implementation but before evidence capture; neither validation nor reviewer
  responses are fabricated. The harness verifies same-session repair feedback.
  Fault injection and session gates also have deterministic unit tests.
- Smoke tests never change credentials or elevate permissions automatically.
  Unsafe inherited Git overrides are rejected; setup disables Git templates and
  hooks. Documentation describes usage cost, limits, provider session retention,
  explicit configuration, and redacted test diagnostics.
- CLI results now have a version-1 delivery envelope with random task/run IDs;
  repair attempts share those IDs. No correlation or logging dependencies were
  added to the workflow/store packages. `log_format` is a configurable text/JSONL
  setting, with UTC timestamps in version-1 JSONL logs. Logging uses allowlisted
  metadata and redacts unknown strings, excluding all free-form task, repository,
  credential and provider data. Full result evidence remains unredacted and must
  be handled as sensitive. Short/failed log or result writes cannot return success.
  Tests cover correlation, concurrent records, redaction, quiet mode, configuration
  precedence and output failures.
- README and operating guides cover installation, statuses, permission boundaries,
  no-rollback/no-durable-resume semantics, recovery artifacts, configuration/wire
  versioning, and known limitations. Approval is not a guarantee of perfect code.
- Live preflight on 2026-09-04 found Codex CLI 0.136.0, OpenCode 1.18.23, a Codex
  ChatGPT login and OpenCode Go credentials. No OpenCode default model was configured.
  Codex's real process-timeout and cancel-after-output probes passed with the
  configured default planner settings. They stop during startup and do not prove
  authenticated model completion or end-to-end approval. Both full live workflows
  and OpenCode lifecycle probes remain pending the user's model selection.
- Provider handling now lives in a shared agent adapter, with safe structured
  `store.ProviderFailure` diagnostics. Explicit billing errors override transient
  indicators; bare 429 remains unknown and non-retryable. A private child context
  stops observed failures without converting them into user cancellation. Partial
  repository evidence remains available and no downstream agent is called.
- The workflow owns retry policy, invocation accounting and cancellation-aware
  waits through a small injected port. Retries default to zero and only apply to
  transient planning/review failures with unchanged repository evidence. Backoff
  is bounded with jitter and honors the longest exposed Retry-After; malformed,
  overflowing or excessive waits fail closed. Mutating calls never auto-replay.
- The default limit is 64 agent invocations per run, including retries. It is not
  a dollar/token cap or a count of hidden CLI HTTP requests. Nonzero
  `execution.max_cost_microusd` is rejected before any agent starts, because these
  CLI adapters cannot enforce authoritative spend. Provider billing controls or
  a metered gateway are required for a guaranteed cap. No automatic credential/model
  fallback, credit purchase, durable checkpoint/resume or cross-run billing ledger
  was added. The confirmed billing-handoff extension below is a separate policy.
- Godog v0.16.0 is a test-only dependency. Strict Gherkin scenarios in
  `cmd/multiharness/features` exercise the real composition with fixture processes,
  Git, validation and parsers. They cover the main workflow and provider failures
  without live model calls. Undefined/pending steps fail; each scenario gets an
  isolated checkout and checks user-note preservation and diagnostic redaction.
  Repair fixtures verify original intent, structured feedback and session reuse.
  Commercial service-level spend accounting, durable recovery and tenant isolation
  remain separate requirements, not guarantees of these local tests.

### Phase 9 completion gate

Confirmed billing-handoff extension:

- The workflow owns a small human-approval port, alternate role ports and immutable
  route identities. CLI terminal I/O stays outside the core. Only `yes` confirms;
  piped input, refusal, blank input, EOF and unavailable terminals never authorize
  a switch. Whole-run cancellation/deadlines bound terminal waits without leaked
  input goroutines. Mode is configurable as `prompt` (default) or `disabled`.
- A confirmed role stays on its alternate for the run; implementation includes
  later repairs. Each role switches once at most, so exhausted alternate billing
  cannot cause ping-pong. Total invocation accounting never resets. Evidence is
  inspected before/after consent; protected changes and stale work cannot be
  authorized away. Partial edits, original plan and feedback are handed forward;
  cross-provider session IDs are cleared. Normal validation/review gates remain.
- Codex now supports ephemeral workspace-write implementation/repair. OpenCode
  supports fresh planning/review sessions with unique deny-by-default runtime
  agents, read-only source tools and external plugins disabled. Managed settings
  remain an operator trust boundary; this is not an OS sandbox. Existing inline
  provider settings are preserved in the child configuration, not printed or
  written globally. Alternate models, executables, timeouts and role settings are
  independently configurable. No real provider calls were used for these tests.
- Shared prompts and versioned response schemas/parsers moved to the outer
  `internal/adapter/agent/structured` package to prevent cross-agent contract drift.
  CLI results optionally record `agent_switches`, and lifecycle logs include
  `agent_switched`. Interactive questions are human-readable stderr even when
  lifecycle logging is JSON; unattended JSONL consumers should disable fallback.

Release-hardening follow-up:

- Shared response decoding now rejects duplicate keys at any depth, escaped-key
  collisions, invalid UTF-8 bytes, noncanonical schema key casing and excessive
  nesting before typed decoding. Regression tests demonstrated the earlier
  acceptance bug, including a repeated blocking finding becoming approval. Wire
  versions are unchanged: only malformed/schema-invalid responses are rejected.
- Live billing tests inject only the unavailable call's billing failure and
  test-owned source faults. All other agent/validation results must be real.
  Separate smoke opt-ins authorize one expected test-scripted yes per route;
  unexpected or repeated requests fail. Production consent remains terminal-only.
  The harness forces zero retries, at most eight launches and bounded timeouts;
  models/permissions remain explicitly configured. Offline harness tests use
  deterministic ports with real CLI/Git/Go validation, not authenticated models.
- `make check` forces live calls off and runs formatting/module checks, unit and
  strict acceptance tests, race tests, vet, build and bounded provider fuzzing.
  CI is configured for Linux/macOS with read-only permissions and immutable action
  revisions. Its security job uses pinned govulncheck/actionlint tools. Publishing
  the repository triggers configured CI; verify the exact published commit's job
  results before treating remote checks as release evidence.
- Go 1.26.6 and x/sys 0.44.0 replace versions flagged by vulnerability scanning.
  The previous scan found no reachable vulnerable calls but did report known
  package/module advisories. The post-update production/test scan reports no
  vulnerabilities. This is a point-in-time gate, not certification.
- `docs/release-readiness.md` distinguishes the remaining live Phase 9 gate from
  durable recovery, authoritative monetary budgets and hosted customer isolation.
  Those commercial infrastructure features remain unimplemented and require an
  agreed local-versus-hosted deployment model; no persistence/tenant architecture
  or spend guarantee is implied by the new tests.

- [ ] The end-to-end workflow is reproducible in a disposable repository and failures are reported without falsely claiming approval.
- [x] `go test ./...` and `go vet ./...` pass.

## Current progress

- Current phase: Phase 9 in progress — local implementation verified; live release gate pending successful authenticated workflows and provider availability.
- Last completed phase: Phase 8 — Configuration and CLI vertical slice, including answer-only routing.
- Architecture update: removed Genkit at the user's request. The existing Go state machine, explicit contract validation, cancellation, repository safeguards, repair limits, and structured events remain. Framework-specific tracing and runtime schema generation are no longer provided; future transports must decode their typed inputs explicitly. The service normalizes empty evidence collections for stable JSON output without manufacturing missing evidence.
- Last verified commands: `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go vet ./...`, `go build ./...`, strict `TestAcceptanceFeatures`, a bounded provider-classifier fuzz run, `gofmt`, and `git diff --check`. The production dependency graph excludes Cucumber/Godog; workflow/store retain no vendor/OS adapter imports. Earlier opt-in Codex startup timeout/cancel probes passed, but full authenticated workflows and OpenCode lifecycle probes have not run. No live model calls were made for the provider-hardening work.
- Architecture-update completion gate: runtime adapter and obsolete step-runner abstraction removed; real-repository integration test migrated to the service; contract validation, output JSON shape, caller-context propagation, ordered events, repair behavior, and repository safeguards verified. The workflow and store packages still import no adapters, filesystem APIs, or vendor SDKs.
- Latest release-hardening verification: `make check`, `make acceptance` (54 strict Cucumber scenarios / 390 steps), `govulncheck -test -show verbose ./...` (no vulnerabilities), workflow actionlint, Linux/amd64 cross-build and whitespace checks passed on Go 1.26.6 / macOS arm64. Four billing-handoff harness paths passed offline. No paid model calls were made, and no remote CI jobs were executed.
- Calculator-demo follow-up: `TestSmokeCalculator` uses existing answer-only adapters, an empty temporary working directory, forced read-only permissions, three calls maximum, and safe provider-failure categories. It prints code without saving or executing the app; CLI temporary/cache/session activity still occurs. The CLI package tests, opt-out skip, vet and formatting checks passed with live calls disabled. The user's live demo reached Codex planning and OpenCode generation, but has not yet completed successfully; it does not replace browser testing or the production workflow release gates.
- Runtime-recovery follow-up: production Codex roles and the calculator demo now use the outer-adapter `RuntimeRunner`. The default bare `codex` is resolved against bounded shared-cache writer metadata, installed stable CLI versions and required flags. Explicit paths/custom commands remain pins. Discovery never calls a model, changes credentials/cache/global configuration, installs packages, or replays task execution; failures stay separate from billing fallback. CLI notices contain a selected numeric version only, remain valid JSONL and fail closed on output errors. The core workflow/store remain unchanged.
- Runtime-recovery tradeoff: cache writer version is a conservative floor, not proof of complete API/model compatibility. Unknown cache metadata/prereleases fail closed for automatic discovery; explicit executable pins opt out. Fixed two-second probe and ten-second preflight safety caps apply inside the configurable stage/whole-run deadlines, with eight candidates maximum. This deliberate additional cap bounds local discovery; managed slow-start installations can use an explicit pin. Automatic installation/update distribution remains separate work requiring a vetted policy.
- Runtime-recovery verification: full offline `go test -count=1 -timeout 10m ./...`, race tests for Codex/CLI/composition packages, `go vet ./...`, Linux/amd64 cross-build, gofmt and whitespace checks passed. The opt-in metadata-only `TestInstalledRuntimeSelection` selected `/Applications/ChatGPT.app/Contents/Resources/codex` version 0.153.0 on this Mac over terminal 0.136.0, without a model request. No paid calculator retry was run during this change.
- OpenCode response-format follow-up: the user's September 4 calculator failure was traced to Big Pickle wrapping the expected final JSON in a Markdown `json` fence. The OpenCode adapter now removes one complete JSON-labelled or unlabelled fence before unchanged strict parsing, preserving embedded code without retrying a model call. Extra commentary, multiple/incomplete fences, malformed JSON and invalid contracts remain errors. A single table-driven regression test exercises 80 cases across planning, review, implementation and repair. `make check` passed (full offline unit/acceptance and race suites, vet, build, module/format checks and bounded provider fuzzing); the calculator opt-out skip also passed. No paid calls were made for this fix, and successful live calculator review remains unverified.
- CLI presentation follow-up: optional `color` (`auto`/`always`/`never`) and `progress` (`auto`/`plain`/`off`) use normal configuration precedence. Text terminals show fixed coloured labels, a width-bounded live line, elapsed/last-activity timing, retry countdowns, repair/provider-switch labels and final evidence counts. Redirected auto output retains text logs; JSON never contains animation or colour. `NO_COLOR`, `TERM=dumb`, CI, quiet/off settings and output failures are handled explicitly. Billing confirmation pauses rendering without changing consent policy; Ctrl+C stops the worker and existing process-tree cancellation remains authoritative.
- CLI activity architecture/tradeoff: an outer process-runner decorator observes bounded Codex/OpenCode stdout metadata for all primary and alternate roles. Only allowlisted labels reach a one-slot, latest-wins presentation mailbox; provider readers never wait on terminal I/O. A 250 ms display tick coalesces activity, so it is best-effort telemetry, not an audit trail or completion evidence. Raw commands, paths, messages, reasoning, sessions and diagnostics are never displayed. Workflow policy is unchanged; its only new metadata is the already-chosen retry delay. No WebSocket, UI framework, credential change, model probe or additional agent launch was introduced.
- CLI presentation verification: `make check` passed, including the full race suite and 58 strict Cucumber scenarios / 423 steps. Follow-up CLI/activity race tests, full offline tests, static checks and Linux/amd64 build passed. A real-terminal run using only fixture binaries completed a repair cycle with clean redraws and a final summary; a second fixture run exited 130 on Ctrl+C with the live line cleared. The final-output failure path now emits a safe failure notice if stderr remains available. No authenticated model calls or remote CI runs were made, and Phase 9's live release gate remains pending.
- Next action: confirm usable primary/alternate provider access, run both live workflows, all four alternate-role handoffs and OpenCode timeout/cancellation probes in disposable repositories, then verify CI on the release commit. Address failures before marking Phase 9 complete; do not infer approval from skipped tests or exhausted repairs. Review the local-versus-hosted deployment boundary before implementing the separate commercial infrastructure requirements.
