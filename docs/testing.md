# Verification and real-agent smoke tests

## One simple live calculator test

For a quick text-only demo, run just this test from the project root:

```sh
MULTIHARNESS_SMOKE=1 MULTIHARNESS_SMOKE_MODEL='opencode/big-pickle' go test -count=1 -timeout 20m -v ./cmd/multiharness -run '^TestSmokeCalculator$'
```

It makes at most three real CLI calls: Codex writes a short plan, OpenCode returns
a standalone HTML/CSS/JavaScript calculator, and a fresh Codex call reviews the
code as text. The generated HTML is printed in the terminal, not saved or run.
No billing simulation, automatic fallback, repair loop, Git setup, or browser is
involved. A rejected review fails the test and prints the findings.

OpenCode models sometimes wrap the requested JSON in a Markdown `json` code
fence. The adapter accepts one complete JSON (or unlabelled) fence around a valid
response without another model call. Malformed JSON, extra commentary and schema
violations still fail. Offline regression coverage exercises this handling for
planning, implementation, review and repair, including preservation of embedded code.

Both agents target an empty temporary directory, which must stay empty. Codex
uses its read-only sandbox; OpenCode uses the existing read-only agent with shell
and edit tools denied and external plugins disabled. Prompts prohibit all tool
use. Model/reasoning/variant settings come from the normal smoke configuration,
but extra arguments are ignored and read-only permissions are forced. Existing
CLI authentication is used; neither credentials nor global settings are changed.
Go and the CLIs still use temporary files, caches/logs and provider session storage;
this is **no app/project-file changes**, not zero disk activity or an OS sandbox
for OpenCode. Managed provider settings remain a trust boundary.

The test uses the same automatic Codex runtime selection as production. The
normal command above needs no PATH override when a compatible CLI is already
installed. It prints the selected version before the Codex call. Version/help
preflight probes are additional local processes, not model calls. To inspect
selection alone without generating code or sending a model request:

```sh
MULTIHARNESS_RUNTIME_CHECK=1 go test -count=1 -v ./internal/adapter/agent/codex -run '^TestInstalledRuntimeSelection$'
```

Use a model you can access with available usage. The command selects Big Pickle
for OpenCode, not the exhausted OpenCode Go model. Codex uses the configured
planner/reviewer models (defaults: `gpt-5.6-sol`, `xhigh`). Provider failures show
the affected stage/model and safe category, such as `billing_exhausted`; raw
provider diagnostics stay private. There are no harness retries or provider
switches. Individual CLIs may make multiple internal requests; this is not a
monetary cap. Each call defaults to five minutes, with a fifteen-minute total cap.

This is a live connectivity/code-generation demo with static AI review, **not**
proof of browser correctness or the production editing/review/repair workflow.
It skips without `MULTIHARNESS_SMOKE=1` and refuses to run in CI. Existing offline
regression tests and the advanced smoke tests below remain available separately.

## Deterministic suite

Run the repeatable local gate with `make check`. It disables live smoke tests and
installed-runtime probes even
when the shell has inherited opt-in settings. The equivalent core commands are:

```sh
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
go build ./...
git diff --check
```

With `MULTIHARNESS_SMOKE` and `MULTIHARNESS_RUNTIME_CHECK` unset, tests use fake ports or fixture subprocesses;
they do not invoke live models. The composition-root integration tests use real
Git, process execution, configuration, parsers, validation and workflow policy.
They cover repairs, exhausted repairs, missing executables and answer-only routing.

`make check` also checks formatting, module tidiness/checksums and bounded provider
fuzzing. A production import-graph test prevents workflow/store from depending on
outer adapters/OS APIs and prevents test frameworks or Genkit entering the binary.
`make security lint-workflows` fetches pinned verification tools, scans production
and test dependencies, and validates the GitHub workflow. These commands need
network access for downloads and the vulnerability database, but never models.

The GitHub workflow runs deterministic checks on Linux and macOS with time limits,
read-only permissions, pinned actions, no saved checkout credentials and no provider
secrets. Live smoke tests reject a nonempty `CI` environment; there is deliberately
no CI path for authenticated model execution. Configuring this workflow does not
prove the remote jobs have run; verify them on the published commit before release.

## Cucumber / Gherkin acceptance tests

The official [Godog Cucumber implementation](https://github.com/cucumber/godog)
runs the `.feature` files in `cmd/multiharness/features` through `go test`:

```sh
go test -count=1 -v ./cmd/multiharness -run '^TestAcceptanceFeatures$'
```

The suite is strict: undefined, pending and failed steps fail the test. Scenarios
run in a reproducibly shuffled order, with separate temporary Git repositories,
fixture state and configuration. Sequential execution is intentional because
subprocess fixtures use scenario-scoped environment variables. Production workflow
policy is not mocked: tests use the real CLI composition, adapters, process runner,
parsers, Git snapshots and deterministic validation, with local fixture agents.
No credentials are read and no live model calls are made by these fixtures.

Coverage includes normal approval, review/repair/revalidation, exhausted and zero
repair limits, answer-only routing, rejection of approval over failed validation,
billing at all four agent stages, terminal authentication/access errors, ambiguous
429, transient recovery and exhaustion, opt-in retries, partial mutations, missing
sessions, exit-zero errors, cancellation of a hanging billing-error process,
Retry-After bounds, invocation budgets and unsupported monetary-cap rejection.
Every executed scenario checks preserved user notes and absence of a synthetic
provider-secret canary in final output/logs. Approval scenarios verify independent
file attribution against deliberately false agent-reported filenames.

Focused unit tests cover provider classification, bounded streaming, backoff,
cancellation, deadline and overflow boundaries, configuration precedence and
contract serialization. Run the bounded fuzz check without provider access:

```sh
go test ./internal/adapter/agent/provider -run '^$' \
  -fuzz '^FuzzClassifyNeverLeaksRawErrors$' -fuzztime=5s -parallel=2
```

Godog and Gherkin are test-only imports; they are not included in the production
binary's dependency graph. Unit, acceptance, race, vet and build checks above are
the local deterministic gate. They do not establish multi-tenant isolation,
authoritative spend accounting, durable restart behavior, or live CLI compatibility.

The billing-fallback extension adds consent scenarios for all four roles,
refusal/blank/EOF, partial implementation, exhausted alternate billing, disabled
fallback and transient errors that must not switch. Fixture processes speak both
protocols in their alternate roles. Unit tests additionally verify terminal-only
input, bounded/cancellable reads, session isolation, role stickiness, unchanged
evidence around consent, restricted commands and preservation of inline provider
configuration. No live authenticated billing failure is required to run this suite.

Shared response-parser tests run through both adapters; to measure their combined
coverage, use `go test -coverpkg=./internal/adapter/agent/structured ./internal/adapter/agent/...`.
The tests do not claim every possible production failure is covered. Authenticated
compatibility for the new alternate roles remains a live release gate too.

The suite now includes 58 scenarios / 423 steps. Cases cover partial repairs,
read-only mutation before consent, exhausted launch budgets, and ambiguous response
keys at every agent stage. Parser unit tests additionally cover escaped duplicate
keys, nested review findings, invalid UTF-8, case collisions and nesting limits.
The alternate-role live harness itself is tested offline with deterministic agent
ports plus real CLI, Git and Go validation. See [release-readiness.md](release-readiness.md)
for the latest verification results and outstanding gates.

CLI progress regressions also cover terminal/pipe/CI/`NO_COLOR` behaviour, plain
and JSON output, elapsed-vs-activity timing, retry countdowns, fallback labels,
width changes, bounded coalescing, consent-prompt pause/resume, cancellation and
writer failures. Four Gherkin scenarios exercise readable repair output, activity
from both fixture agents, JSON escape safety and progress suppression through the
real composition. They make no authenticated model calls. The calculator smoke
test uses Go's test logger; run the actual CLI to see the interactive renderer.

## Live opt-in

First install/authenticate the CLIs independently and inspect their permissions.
Do not put credentials in test flags or configuration files. Confirm an available
OpenCode `provider/model`; tests require an explicit selection so they do not
silently consume an unrelated default provider. Existing CLI credentials and
provider usage are used; model calls can cost money or consume subscription quota.

```sh
MULTIHARNESS_SMOKE=1 \
MULTIHARNESS_SMOKE_MODEL='your-provider/your-model' \
go test -count=1 -timeout 45m -v ./cmd/multiharness \
  -run '^(TestSmokeWorkflow|TestSmokeAgentCancellation)$'
```

Replace the placeholder; it is not a model recommendation. Optional controls:

- `MULTIHARNESS_SMOKE_CONFIG`: explicit version-1 configuration file for agent
  executables/models/reasoning/variant/permissions and Git settings.
- `MULTIHARNESS_SMOKE_MODEL`: overrides the file's implementer model.
- `MULTIHARNESS_SMOKE_STAGE_TIMEOUT`: positive duration for each agent invocation;
  defaults to five minutes. Each full workflow is capped at twenty minutes.

The harness forces zero provider retries and at most eight whole-agent launches
per workflow, even if the application config permits more. This is not a monetary
cap. Ordinary live scenarios disable billing fallback: an unexpected real billing
failure stops and cannot silently consume another provider's credits.

Other application `MULTIHARNESS_*` variables are deliberately ignored by this
harness. Inherited `GIT_*` overrides are rejected because they can redirect agent
commands outside the disposable checkout; only `GIT_TERMINAL_PROMPT=0`, which Go
sets to prevent credential prompts, is allowed. Clear any other such variables
for the test command only. The harness replaces the target with a disposable Git
repository, installs its own Go validation command, and uses zero repairs for
immediate approval or one repair for the repair test. It does not run in the
project checkout, clone user files, change credentials, or elevate permissions.
The default is still `reject_on_prompt`; if permissions block a run, inspect the
failure and choose an appropriate scoped policy yourself. The test does not retry
under `auto_approve` or a different model.

The tests run sequentially and require real responses:

1. **Immediate approval:** fix a tiny Go addition function, independently run
   tests for positive/negative/zero inputs, then obtain actual Codex approval.
2. **Repair loop:** after the first actual OpenCode implementation, a test-only
   decorator deliberately changes the function to subtraction. This occurs before
   evidence capture. Real validation must fail, Codex must reject the change,
   and OpenCode must repair it using the same session and structured feedback.
   A second real validation/review must approve. No agent response is stubbed.
3. **Timeout/cancellation:** invoke each real adapter with a short process timeout,
   then separately cancel only after the real process emits output. Assert the
   error preserves deadline/cancellation semantics and terminates within bounds.
   The short-timeout probe can expire during CLI startup; it does not claim to
   interrupt a running model tool. Process-tree behavior is separately tested by
   deterministic Unix subprocess tests.

### Live billing handoffs

This is a separate explicit opt-in. Choose both the primary and alternate OpenCode
models, or configure their respective roles in `MULTIHARNESS_SMOKE_CONFIG`:

```sh
MULTIHARNESS_SMOKE=1 \
MULTIHARNESS_SMOKE_FALLBACK=1 \
MULTIHARNESS_SMOKE_MODEL='your-primary-provider/your-model' \
MULTIHARNESS_SMOKE_FALLBACK_MODEL='your-alternate-provider/your-model' \
go test -count=1 -timeout 90m -v ./cmd/multiharness \
  -run '^TestSmokeBillingFallback$'
```

The fallback model variable applies to OpenCode planning and review; their separate
config fields remain available. Codex implementation uses the configured
`fallback.codex_implementer` model, reasoning and `workspace-write` settings.
Running this command authorizes exactly one test-scripted `yes` for each expected
route. A second or different route is refused. This is test-only input injection,
not an unattended production consent flag. Ordinary terminal-consent behavior is
covered by the unit and acceptance suites.

Four disposable workflows inject a billing error at planning, implementation,
review and repair respectively. The unavailable call is replaced by that error;
the alternate and all remaining stages run real agents and validation. Tests never
deplete an account intentionally. The implementation case leaves a partial source
edit for Codex to inspect, then forces a later repair on Codex. The review case
requires OpenCode to reject failing validation, then approve a real same-session
OpenCode repair. The repair case starts with an OpenCode session and verifies it
is cleared before Codex receives the feedback. All cases require actual approval,
exact invocation accounting, original-baseline attribution and unchanged fixtures.

Codex's ephemeral and workspace-write assertions follow its
[documented non-interactive behavior](https://learn.chatgpt.com/docs/non-interactive-mode).
Passing these tests establishes alternate-role compatibility, not every provider's
real billing-error format; fixture error-stream tests cover those normalized paths.
Use a subtest suffix such as `/repair$` to run just one route while investigating.

The harness checks independent changed-file attribution, preserved user notes
and unchanged tests. Temporary checkouts/config files are cleaned up by Go tests.
CLIs may still retain their own sessions/logs according to their settings.
Failure output reports stage/code and run ID, withholding raw provider diagnostics
from test logs. To diagnose an adapter failure, reproduce it in another disposable
repository with the normal CLI and inspect its full result securely; never paste
auth files or unredacted provider logs into an issue.

Passing deterministic tests or skipped smoke tests does **not** satisfy the live
release gate. Record the date, OS, CLI versions, configured models/variants,
permission policy and actual outcomes. Approval once is compatibility evidence,
not a guarantee that stochastic agents never fail.

## Current live-verification status

Preflight on 2026-09-04 found Codex CLI 0.136.0 and OpenCode 1.18.23. Codex reported
an existing ChatGPT login; OpenCode listed existing OpenCode Go credentials.
No OpenCode default model was configured. Codex's real process-timeout and
cancel-after-output probes passed, using the default planner configuration
(`gpt-5.6-sol`, `xhigh`, read-only). These probes stop during startup and do not
prove a completed model response or authenticated end-to-end approval.
The OpenCode model selection, its cancellation probes, both normal live workflows,
and the four real alternate-role handoffs are pending; Phase 9 is not yet
release-approved. No authenticated calls were made during this hardening pass.
