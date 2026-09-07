# Test coverage audit

Measured on 2026-09-07 against the working tree based on
`d1c109040b18d5e8818b693416f05f33237b5b71`, including existing local changes and
downloadable-release preparation. Before and after measurements used Go 1.26.6,
Linux amd64, CGO enabled and GCC in an isolated Docker container. Both ran
`go test -json -count=1 -timeout 5m -coverprofile=... ./...`, with live-provider,
installation and installed-runtime probes disabled.

## Assessment

The offline suite covers the main local-operator workflow well. Most tests should
remain: they protect planning decisions, implementation/session handoffs,
independent review, repairs, provider failures, cancellation and existing user
files. Repository locks also protect a single operator from accidentally launching
two runs; they are not an unnecessary multi-tenant feature.

The clear duplication was in OpenCode response handling. This audit:

- Reduced the four-role formatting matrix from **80 to 32 cases**. Each role
  still accepts valid bare/fenced output and rejects duplicate keys, unsupported
  schemas and invalid domain values. The detailed fence variants run once through
  the public implementation adapter, which shares fence handling with other roles.
- Removed three redundant private-parser/error-wrapper tests. Public execution
  tests already verify successful results, session capture and inspectable provider
  failures. Missing/null implementation fields and blank changed-file cases live
  with the shared structured parser, alongside its existing strict JSON tests.
- Retained event ambiguity, session mismatch, permission, context handoff,
  cancellation, preservation and false-approval regressions. No production
  behavior or live smoke harness was changed for this audit.

Test entry points fell from **205 to 202**, plus one fuzz target, across the same
62 test files. Physical test-source lines fell from 9,725 to 9,670. These counts
include live opt-ins and subprocess fixture entry points; they exclude `TestMain`
and test functions inside generated-source strings. Many lines are fixture setup
and helpers, so file or line count alone is not a useful deletion target.

## Measured coverage

| Area | Before this audit | After cleanup |
| --- | ---: | ---: |
| **All packages, statement-weighted** | **81.5%** | **81.5%** |
| Workflow core | 92.2% | 92.2% |
| Provider failures | 94.3% | 94.3% |
| Codex adapter | 78.4% | 78.4% |
| OpenCode adapter | 86.6% | 86.2% |
| Shared structured responses | 68.0% | 68.0% |
| Store contracts | 80.9% | 80.9% |
| Composition root | 57.4% | 57.4% |
| Configuration | 91.2% | 91.2% |
| CLI transport | 73.5% | 73.5% |
| Agent activity | 58.9% | 58.9% |
| Process execution | 88.2% | 88.2% |
| Dependency setup | 83.0% | 83.0% |
| Deterministic validation | 96.7% | 96.7% |
| Git workspace evidence | 82.5% | 82.5% |

Covered statements changed from **3,076/3,772 to 3,075/3,772**. The only lost block
is the private implementation parser's defensive empty-session rejection. The
event stream rejects missing sessions before that parser is reached, and its
missing-session regression remains. No other previously covered block was lost.
The workflow's `Run` and stage-sequence functions retain 100% statement coverage.

These figures instrument each package for its own test binary. Shared prompt
builders, for example, are exercised by adapter/context tests but do not receive
credit in the structured package's own percentage. `-coverpkg=./...` measures
cross-package execution separately and should not be compared directly with this
table. Statement coverage is neither branch coverage nor proof that assertions
detect every defect. This audit did not perform mutation testing.

The September 5 macOS measurement of 81.8% covered an earlier source tree. It is
historical evidence, not a before/after comparison for this audit. A Windows
baseline on the current tree passed at 69.3%, with workspace and integration
platform skips; that is not representative of supported Linux/macOS execution.

## Verification and remaining limits

The reduced Linux suite passed, including production-composition integration.
`make fmt`, `make static`, `make race` and `make fuzz` passed on Go 1.26.6.
The container ran as root, so its full-suite permission regression skipped;
`TestWorkspaceRequiresWritePermission` then passed separately as a non-root user.
The ordinary Linux skips otherwise covered explicitly opted-in live model and
installed-runtime probes. The changed adapter/parser packages also passed on
Windows with Go 1.26.6, and the host working-tree whitespace check passed.

Uncovered code includes startup/error paths, presentation, default retry-timer
waiting, actual installed-runtime discovery and terminal installation wiring.
Existing deterministic tests cover retry policy and cancellation via an injected
waiter; they do not establish actual provider timing. Fresh-host setup,
authenticated approval/repair/cancellation/fallback, native macOS execution of this
tree, and remote CI remain separate [release gates](release-readiness.md). Native
Windows workflows remain intentionally unsupported. No provider or installer was
called during this audit.

Context tests verify complete repair inputs after simulated loss of harness
history. There is no application-owned compactor, and native Codex/OpenCode
compaction was not exercised.

## Reproduce

On a supported Linux/macOS host with Go 1.26.6 and race-detector prerequisites:

```sh
make fmt
make coverage
make static race fuzz
```

`make coverage` forces live calls off and writes ignored local reports:

- `.coverage/coverage.out`: statement coverage profile.
- `.coverage/index.html`: source highlighting for covered and uncovered code.

`go tool cover -func=.coverage/coverage.out` prints the function summary without
rerunning tests. This audit's local before/after profiles and annotated report are
under `.coverage/linux-before/` and `.coverage/linux-after/`. Test ownership and
commands are in [testing.md](testing.md).
