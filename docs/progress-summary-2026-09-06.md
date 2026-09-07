# Multiharness Core — progress through 6 September 2026

Phases 1–8 are complete. Phase 9 is in progress: the local workflow, safety
controls, CLI and offline verification are implemented, while authenticated
end-to-end runs and the remaining release checks are still pending.

This summary reflects the repository checklist, operating guides and commit
history at `21884dd` (`Refactor workflow files for a clearer execution flow`,
5 September 2026). Verification results below are recorded results from that
work; no new tests or live model calls were run to prepare this summary.

## What we built

Multiharness accepts a task and coordinates planning, implementation,
deterministic validation, independent review and repairs. Coding runs finish
with an explicit approval or terminal failure condition. Non-coding requests
can receive a direct planner answer through a separate `answered` branch that
skips implementation, validation and review.

The default planner and reviewer use Codex with `gpt-5.6-sol` and `xhigh`
reasoning. OpenCode implements changes and handles repairs. Planning and simple
answers can explicitly select OpenCode. Commands, models, variants, reasoning,
permissions, timeouts, validation commands and repair limits are configurable.

| Completed phase | Delivered capability |
| --- | --- |
| 1 — Foundation | Go module, package boundaries, process execution and initial workflow contracts |
| 2 — Workflow contract | Validated task/plan/result types, review findings, repair requests and explicit terminal statuses |
| 3 — Process hardening | Stdin prompts, environment overrides, bounded output, progress, timeouts, cancellation and process-tree cleanup on supported systems |
| 4 — Orchestration | Injected workflow service, ordered stages/events, review/repair loop and caller-context propagation |
| 5 — Codex roles | Read-only planning/review, schema-constrained output and independent reviewer execution |
| 6 — OpenCode implementation | Implementation and repair adapters, JSONL events, session capture/resumption and explicit permission behavior |
| 7 — Evidence and validation | Original Git baseline, independently attributed changes, protected user files, workspace locking and configurable checks |
| 8 — Configuration and CLI | Production composition, configuration precedence, task input, structured results, progress, exit codes and answer-only routing |

## Architecture and repository safeguards

- The application now uses one plain-Go `workflow.Service`, constructed at the
  composition root and called through `Service.Run(ctx, input)`. Genkit and
  redundant state, mediator and strategy scaffolding were removed.
- Workflow policy depends on small, consumer-owned ports. Serializable
  contracts live in `internal/store`; process, Git, validation and agent
  implementations stay in outer adapters. Configuration and CLI presentation
  remain outside the core.
- Every run acquires a workspace lease and records the original repository
  baseline before agents run. Observed changes replace agent-reported file
  claims, and review receives repository and validation evidence.
- Pre-existing dirty paths are protected at whole-file granularity. Changes to
  protected paths, the index or HEAD stop the run. Preservation failures retain
  recovery evidence; there is no automatic rollback, staging or commit.
- Planning, validation and review must preserve the inspected checkout.
  Incomplete or stale evidence cannot produce approval. Unsupported repository
  layouts and incomplete snapshots fail explicitly.

## Phase 9 work completed locally

- Added structured task/run correlation IDs, versioned JSON delivery and
  redacted lifecycle logs. Full result evidence remains sensitive.
- Normalized billing, rate-limit, overload, authentication, access and unknown
  provider failures. Reported failures stop promptly, including failures before
  session creation or inside an exit-zero response.
- Added optional bounded retries for eligible read-only planning/review
  failures, plus per-run invocation limits. Implementation and repair are never
  automatically replayed.
- Added billing-only role fallback with explicit terminal consent, safe
  cross-provider context handoff and provider session boundaries. Refusal, EOF,
  non-interactive execution and alternate-provider failure have offline coverage.
- Added opt-in live workflow, lifecycle and four-stage billing-handoff smoke
  harnesses, plus a text-only calculator demonstration.
- Hardened structured responses against duplicate or noncanonical keys,
  ambiguous nested JSON and invalid UTF-8. OpenCode accepts one complete JSON
  Markdown fence before the same strict parsing and contract validation.
- Added bounded offline selection of a compatible installed Codex runtime when
  the default CLI is older than the shared model cache. Explicit executable
  pins remain authoritative.
- Added terminal colours, safe live activity, elapsed/retry timing, repair and
  provider-switch labels, and final evidence counts.
- Added consented, bounded missing-dependency installation handling with pinned
  packages and actionable unattended failures. Successful setup requires a new
  invocation after sign-in; it never replays the task.
- Documented installation, configuration, statuses, trust boundaries, recovery,
  schemas, provider failures, testing and release requirements.
- Added repeatable release checks, production dependency-boundary checks,
  vulnerability scanning and pinned Linux/macOS CI configuration. Updated the
  Go toolchain to 1.26.6 and `x/sys` to 0.44.0.

## Latest cleanup and fixes

The September 5 cleanup made the runtime easier to read and reduced duplicate
code while preserving the workflow boundaries:

- Renamed the CLI protocol adapters to `schemaexec` for Codex and `sessionexec`
  for OpenCode, and consolidated their files without changing configuration v1
  or adding providers.
- Replaced Cucumber/Gherkin with ordinary Go integration tests through the
  production composition. Removed Godog and seven transitive dependencies.
- Focused tests on planning, implementation, review/repair, complete context
  handoff and provider failures, retaining cancellation and evidence safeguards.
- Removed unused files, forwarding helpers and the unconnected local-model
  placeholder; added `.editorconfig` and consistent `make fmt` formatting.
- Fixed cancellation after final callbacks/cleanup, stale evidence before
  implementation, ambiguous provider/session JSON, hidden terminal consent,
  oversized confirmation input and noncanonical configuration keys.
- Grouped the six stages in execution order in `stages.go`, kept the full
  sequence and repair loop in `service.go`, and reduced workflow production
  files from 18 to 10 and startup/composition files from three to two.

## Verification recorded so far

The latest workflow cleanup records a passing `make fmt check`: offline tests,
full race tests, vet, build, architecture/module/format/whitespace checks and
bounded provider fuzzing. The focused integration suite contains 10 workflow
and 12 provider-failure scenarios using real adapters and fixture subprocesses.

The latest documented coverage measurement, taken before the later adapter
consolidations and formatting pass, is **81.8% overall**, **92.0% workflow** and
**95.0% provider handling**. Test cleanup reduced Go test source from 75 files /
10,391 lines to 62 files / 8,436 lines at that checkpoint. These are historical
measurements, not a fresh measurement of this checkout.

Earlier recorded checks include a vulnerability scan with no findings after
the dependency updates, workflow action linting and cross-platform compilation.
Windows was compiled only; native Windows workflows remain unsupported.

Codex startup timeout/cancellation probes passed. Full authenticated workflow
approval, OpenCode lifecycle probes and the successful completion of the live
calculator demonstration remain unverified. Context tests verify complete
handoff after simulated loss of session history; native provider compaction
has not been tested.

## Remaining work before completing Phase 9

1. Confirm usable OpenCode models/provider access for primary implementation and
   alternate planning/review, with usage authorized.
2. Run real immediate-approval and repair-loop workflows in disposable
   repositories, and complete the real-CLI timeout/cancellation checks.
3. Run all four live billing-fallback paths. Record actual CLI versions, OS,
   models, variants, permissions, outcomes and correlation IDs.
4. Verify installation, permissions, offline-network failures and PATH behavior
   on clean macOS/Linux hosts.
5. Run and inspect deterministic and security CI jobs on the exact release
   commit on Linux and macOS, then review the phase results.

Skipped smoke tests and exhausted repair limits never count as approval.
Remote CI execution and fresh-host installation are still pending in the
recorded release evidence.

Commercial deployment also requires a separate decision about local versus
hosted operation. Durable recovery/resume, enforceable monetary budgets and
hosted customer isolation remain unimplemented. The current application is a
local, single-operator CLI.

## Source records

- [Implementation checklist and detailed progress](../AGENTS.md)
- [Project overview](../README.md)
- [Architecture and reading order](architecture.md)
- [Release readiness and remaining gates](release-readiness.md)
- [Testing and live smoke procedures](testing.md)
- [Recorded coverage measurements](coverage.md)
- [Repository checkpoint](https://github.com/ErOr-0/Multiharness-Core/commit/21884dd)
