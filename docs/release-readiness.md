# Release readiness

## Code review follow-up — 2026-09-05

Fixed final-stage cancellation, stale evidence before implementation, ambiguous
provider/session JSON, hidden terminal confirmation, oversized answer carryover
and noncanonical configuration keys. Removed redundant workspace preflight,
unused session progress/error-text plumbing, repeated guards/state assignments
and test-only helpers from production composition.

`make fmt check` passed: offline tests, full race suite, vet, build, module and
format checks, and provider fuzzing. Linux CLI tests cross-compiled. These checks
used fixture agents and local terminals; the live Phase 9 gates remain pending.

## Execution-adapter consolidation — 2026-09-05

The existing CLI adapters now live in `schemaexec` (Codex protocol) and
`sessionexec` (OpenCode protocol). The latter was consolidated from 16 files to
five source and five test files. Configuration v1, command names, permissions,
session handling and provider-failure semantics are preserved. `make check`
passed after the rename; no live model or remote CI runs occurred.

## File cleanup — 2026-09-05

- Removed eight redundant production files and 99 lines, five empty legacy
  folders, generated coverage reports and the obsolete acceptance target.
- Shared parser tests moved to the shared adapter package. Full offline tests,
  race detection, static checks and provider fuzzing passed; coverage is 81.8%.
- Linux/amd64 and Windows/amd64 builds passed, along with Windows workspace test
  compilation. No Windows execution, live model calls or remote CI runs occurred.

This project is a local, single-operator CLI. It is not yet approved for commercial
release or deployment as a shared multi-tenant service. Passing fixture tests is
not a substitute for authenticated CLI compatibility or infrastructure guarantees.

## Focused test suite — 2026-09-05

- Replaced the duplicate Cucumber suite with ordinary Go integration tests:
  10 workflow and 12 provider-failure scenarios passed with real adapters and
  fixture subprocesses. Godog and its seven transitive dependencies were removed.
- Offline tests, race detection, vet, build, formatting/module checks and bounded
  provider fuzzing passed. Test code fell from 10,391 to 8,436 lines.
- Tests concentrate on planning, implementation, review/repair, complete context
  handoff and provider failures, with supporting execution/evidence safeguards.
- There is no application-owned compaction engine. Context tests simulate missing
  harness history and verify self-contained repair prompts; native compaction is
  not claimed as tested.

## Earlier release-hardening evidence — 2026-09-04

- The former Cucumber suite passed 63 scenarios / 461 steps before consolidation.
- `make check`: formatting, module checks, unit/acceptance, race, vet, build and
  bounded provider fuzz tests passed on Go 1.26.6 / macOS arm64.
- `govulncheck -test -show verbose ./...`: no vulnerabilities found after the
  updates. Workflow actionlint and a Linux/amd64 cross-build also passed.
- New live-handoff harness: all four paths passed offline using deterministic
  agents plus real CLI/Git/Go validation. Authenticated execution is still pending.
- Shared-parser regression tests reproduced duplicate/case/UTF-8 acceptance before
  the fix and passed afterward, including ambiguous nested review findings.
- Go minimum updated from 1.26.5 to 1.26.6; `x/sys` from 0.42.0 to 0.44.0.
- Linux/macOS CI configuration uses pinned actions and disables provider calls.
  The workflow triggers on pushes and pull requests; publishing commits is not
  proof of passing CI. Verify the jobs on the exact commit intended for release.
- Terminal progress: coloured/live and plain/JSON modes, safe activity buffering,
  retry timing, consent pause/resume and output failures passed offline unit/race
  and acceptance checks. Real-terminal fixture runs verified a repair cycle and
  Ctrl+C cleanup (exit 130). No live provider calls were used for this UI work.
- Dependency setup: offline consent, installation failure/cancellation, pins,
  concurrent setup locking, configuration, and missing-CLI workflows passed.
  Native Windows now explicitly rejects workflows until its lifecycle safeguards
  are implemented. No real installation or authenticated model call was made.

## Remaining Phase 9 gate

Dependency setup now has a consented, bounded npm installation path with offline
regression coverage. Real installation, permissions, offline-network and PATH
checks on clean macOS/Linux hosts remain a release gate; no installer was run on
the developer's machine. Native Windows is not production-verified. See
[setup boundaries and failure matrix](setup.md).

1. Operator selects available OpenCode provider/models for primary implementation
   and alternate planning/review and authorizes their usage.
2. Run both `TestSmokeWorkflow` scenarios, OpenCode lifecycle probes, and all four
   `TestSmokeBillingFallback` paths. Record actual CLI versions, OS, configured
   models/variants, permissions, outcomes and correlation IDs. Keep raw results
   private. A skipped test or exhausted repair limit is never a pass.
3. Run the deterministic and security CI jobs on the exact release commit on
   Linux and macOS. Resolve any platform-specific failures.
4. Review the phase results before starting another implementation phase. No tag,
   publish, commit, push or provider-account change is implied by this checklist.

Commands and consent boundaries are in [testing.md](testing.md).

## Commercial requirements beyond the current local CLI

These items remain unimplemented. They need an agreed deployment model and
recovery/spending policy; they are not satisfied by invocation limits or Git locks.

| Requirement | Work needed before claiming support |
| --- | --- |
| Durable recovery/resume | Versioned private checkpoints, original-baseline persistence, atomic durable writes, exclusive recovery ownership, configuration/provider consent binding, repository-drift checks and crash-at-every-boundary tests |
| Interrupted mutations | Mark in-flight implementation/repair as uncertain; require operator reconciliation. Never replay automatically or assert exactly-once external side effects from a filesystem snapshot |
| Enforceable monetary budgets | A provider or metered gateway able to authorize spend before requests, durable reservations/accounting, idempotent reconciliation and concurrent-budget tests. These CLI interfaces cannot guarantee a dollar cap |
| Customer isolation for hosted use | Separate OS/container workers, credentials, provider sessions, storage and authorization per tenant; controlled environment/network/retention and adversarial cross-tenant tests |

First decide whether customers run their own local CLI with their own provider
accounts, or submit jobs to a hosted service. That determines the trust boundary
and where budgets, credentials and persistence belong. Do not expose this process
as a shared service before those boundaries are implemented and verified.

For now, retain the documented manual [recovery procedure](recovery.md), enforce
actual spend through provider controls, and run untrusted tasks only inside an
operator-controlled isolated environment.
