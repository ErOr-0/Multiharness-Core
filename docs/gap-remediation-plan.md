# Gap assessment and remediation plan

Drafted 2026-09-07 against the current working tree, based on HEAD
`d1c109040b18d5e8818b693416f05f33237b5b71` plus existing uncommitted changes.
This is a plan; no implementation phase or release gate is marked complete.

## Confirmed scope

The user confirmed on 2026-09-07 that the project focuses only on a local,
single-operator CLI using the operator's repositories and provider accounts.
This deployment model is intentional and is not a gap to fix.

Hosted services, multi-tenant isolation, centralized customer billing and related
service architecture are outside this plan. They are not prerequisites for the
current local CLI. The remaining work concerns local reliability, provider
compatibility, platform support and release verification.

## Assessment of the five review comments

| Comment | Assessment | Evidence and qualification |
| --- | --- | --- |
| Native Windows execution is disabled | Confirmed product limitation | `internal/adapter/workspace/git/files_windows.go` rejects workspace access. Windows also uses unsupported workspace locking, direct-child-only cancellation and unsupported terminal input/consent adapters. Removing the guard alone would weaken safety. |
| Authenticated approval, repair, cancellation and fallback tests remain pending | Confirmed documented verification gap | `docs/release-readiness.md` and `docs/testing.md` keep these gates open. The smoke harnesses already exist. Historical Codex startup cancellation probes do not prove authenticated completion or OpenCode lifecycle behavior. No new live execution was performed for this assessment. |
| Race tests could not run in this Windows environment | Reproduced environment limitation | Current `CGO_ENABLED=0`; the configured compiler is `gcc`, and `gcc`, `clang` and `cc` were not found on PATH. A race-test startup probe failed with `go: -race requires cgo`. Earlier release notes record successful race runs, so this does not mean the project has never passed race detection. |
| Review used Go 1.27 while the project specifies 1.26.6 | Confirmed local version difference; not proof of a defect | Current toolchain is `go1.27.0 windows/amd64`; `go.mod` declares `go 1.26.6`. That declaration is a minimum requirement, not an exact pin. Testing 1.27 alone does not establish 1.26.6 compatibility for the current tree. Existing CI already selects its version from `go.mod`. |
| Local single operator, not commercial or multi-tenant | Intended scope; no remediation required | The user confirmed local single-operator use. Multi-tenancy is outside scope. Local release readiness is assessed through the remaining platform, setup and validation gates. |

Go documents the [race detector prerequisites](https://go.dev/doc/articles/race_detector#Requirements)
and [minimum-version and toolchain-selection semantics](https://go.dev/doc/toolchain).

## Recommended order

Complete the outstanding Phase 9 validation on Linux/macOS first. WSL is a Linux
deployment path, with Linux-installed tools; native Windows support is a separate
implementation effort. This machine currently lists only the `docker-desktop`
WSL distribution, so a configured Linux development distribution must not be assumed.

Retain the plain-Go `workflow.Service.Run` entry point, workflow-owned ports,
existing provider configuration and repository-preservation policy throughout.
Preserve the current uncommitted changes. Follow the phase completion and progress
review rule in `AGENTS.md` before starting a subsequent implementation phase.

### 1. Establish current baseline evidence

- Use a supported Linux/macOS host, or configure a dedicated WSL development
  distribution. Start with the Go version declared in `go.mod`, currently 1.26.6,
  and the required race-detector prerequisites.
- Confirm `go version`, `go env GOVERSION GOOS GOARCH CGO_ENABLED CC`, Git and
  compiler availability. Record the commit and any working-tree changes with results.
- Make the compiler selection explicit for the validation session. Setting
  `GOTOOLCHAIN=local` alone does not select 1.26.6 if the installed Go is 1.27;
  install/select the intended toolchain or use `GOTOOLCHAIN=go1.26.6` for the run.
  Do not change `go.mod` just to match this workstation.
- Run the existing gates: `make fmt`, `make check`, `make security` and
  `make lint-workflows`. Fix reproduced failures and rerun affected checks.
- Verify existing Linux/macOS CI jobs on the exact release candidate. Retain
  `go-version-file: go.mod`; an additional Go 1.27 compatibility job is optional.

Completion evidence: current offline, integration, race, static, fuzz and security
results on the declared baseline; exact CI job links and commit; skips and failures
reported separately. Historical passes and cross-compilation are not substitutes.

Primary files: `Makefile`, `.github/workflows/check.yml`, `docs/testing.md` and
`docs/release-readiness.md`. Extend their existing checks only where a gap remains.

### 2. Execute the existing authenticated compatibility gates

- Prepare explicit configuration with available primary and fallback models,
  authenticated CLIs, intended permissions and bounded stage/run limits.
  Live execution remains a separately opted-in action; this planning request
  does not initiate provider usage.
- Run `TestSmokeWorkflow`: immediate approval and the injected-fault repair loop
  in disposable repositories. Require real validation, independent Git evidence,
  preserved user notes and the expected implementation-session handoff.
- Run `TestSmokeAgentCancellation` for both CLIs, covering timeout and cancellation
  after output. Distinguish startup termination from cancellation during actual
  agent work. Add a focused live case if the existing probe cannot demonstrate
  the intended lifecycle boundary; retain deterministic descendant-process tests.
- Run all four `TestSmokeBillingFallback` routes: planning, implementation,
  review and repair. Use the existing injected billing failure and scoped scripted
  consent, with real subsequent provider responses. Do not deliberately exhaust
  an account. Separately validate human terminal consent where claimed.
- Cover the supported OpenCode planner selection with explicit configuration;
  the Codex-planner path alone cannot establish both planner configurations.
- Fix compatibility failures in the responsible adapter and add a focused offline
  regression before repeating the affected live scenario.

Completion evidence: CLI versions, OS, Go version, configured role/model/variant,
permissions, commit, scenario outcomes and correlation IDs. Keep raw provider and
repository output private. A skipped case, exhausted repair limit or injected
trigger is never reported as a successful real-provider error reproduction.

Primary files: `cmd/multiharness/smoke_test.go`, `smoke_fallback_test.go`,
`smoke_harness_test.go`, agent adapters and `docs/testing.md`. Reuse these harnesses;
do not introduce a second test framework.

### 3. Close the remaining local-release setup gate

- Exercise installation and setup on clean Linux/macOS environments using the
  existing documented package pins and explicit terminal installation consent.
- Check success, refusal, noninteractive execution, permissions, unavailable
  network, missing PATH entries, timeout/cancellation and restart guidance.
- Confirm installation cannot automatically replay a task or turn partial work
  into approval. Record actual fresh-host outcomes in `docs/setup.md` and
  `docs/release-readiness.md`.

This gate was omitted from the five comments but is already part of Phase 9.
Review Phase 9 evidence before beginning the Windows implementation phase.

### 4. Implement native Windows as a dedicated platform phase

Keep the workspace rejection in place until the replacement safeguards pass.

| Boundary | Implementation work | Required behavior checks |
| --- | --- | --- |
| Process lifecycle | Add a Windows process-tree implementation; adapt the runner lifecycle if reliable child containment needs start/cleanup hooks. Keep OS details in `internal/adapter/process`. | Timeout, cancellation before/after output, child and grandchild cleanup, retained pipe handles, startup failure and bounded return time. Unsupported containment must fail before agent work. |
| Workspace ownership and snapshots | Add Windows locking and safe access/snapshot behavior in `internal/adapter/workspace/git`; update unsupported-platform build tags. | Competing processes, linked worktrees, lock release after exit, protected dirty files, index/HEAD preservation, Windows path casing, symlinks/junctions and file replacement during capture. Unsupported filesystem cases must fail explicitly. |
| Terminal and consent | Add Windows console input, cancellation, terminal detection and billing confirmation in `internal/transport/cli`. | Real PowerShell/Windows Terminal interaction, Ctrl+C while reading/running, EOF, oversized input, redirected streams, CI and prompt-output failures. Invisible or piped input cannot authorize fallback. |
| Private local artifacts | Check permissions and atomic-write behavior for recovery files and personal settings on supported Windows filesystems. | Sensitive artifacts remain private; access/replacement failures preserve existing user data. Unix permission bits alone are not the acceptance test. |
| CLI launch and packaging | Resolve supported native executable paths and wrapper handling; supply a Windows build/install entry point. Keep automatic dependency installation unsupported until separately implemented and tested. | Spaces and Unicode in paths, argument/stdin integrity, cancellation through launch wrappers, missing commands, scripted JSON and interactive `magent` startup. |
| Platform integration | Port the existing production-composition fixtures and Unix-specific test commands; add native Windows CI with race prerequisites. | Approval, answer-only, repair, rejection, provider failure and preservation scenarios actually execute instead of hitting the current Windows skips. Run native tests, not only cross-builds. |

Enable native workspace execution only after these safeguards pass. Replace the
blanket-rejection regression with supported-path tests and specific unsupported-case
rejection tests. Then repeat the relevant authenticated workflow, lifecycle and
fallback matrix on native Windows before claiming provider compatibility there.
WSL remains available while this phase is incomplete.

Primary integration points: `internal/adapter/process/os_runner.go`,
`process_group_other.go`, `internal/adapter/workspace/git/files_windows.go`,
`lock_other.go`, `snapshot.go`, `internal/transport/cli/terminal_other.go`,
`cmd/multiharness/integration_test.go`, `internal/workflow/evidence_integration_test.go`
and `.github/workflows/check.yml`.

## Verification performed for this draft

- Inspected the current platform adapters, smoke harnesses, release notes, CI,
  Make targets, toolchain declaration and local environment.
- Passed `TestNativeWindowsFailsWithoutTouchingUserFiles` on Windows with Go 1.27.
  This verifies intentional rejection and preservation, not Windows workflow support.
- Reproduced the race-detector startup failure using
  `go test -race -run '^$' ./internal/workflow`; no race-instrumented tests ran.
- No full test suite, Go 1.26.6 run, live provider test, installation, remote CI
  execution or commercial-readiness verification was performed for this draft.
