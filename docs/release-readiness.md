# Release readiness

This project is a local, single-operator CLI. It is not yet approved for commercial
release or deployment as a shared multi-tenant service. Passing fixture tests is
not a substitute for authenticated CLI compatibility or infrastructure guarantees.

## Current evidence — 2026-09-04

- Strict Cucumber: 63 scenarios / 461 steps passed with fixture subprocesses.
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
