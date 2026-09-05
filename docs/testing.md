# Testing

The suite uses Go's standard `testing` package. Tests focus on five behaviors:

| Focus | What must hold | Main tests |
| --- | --- | --- |
| Planning | Correct `answer`/`implement` branch, configurable planner, valid plan, read-only execution | `internal/workflow/service_answer_test.go`, `internal/adapter/agent/schemaexec/planner_test.go`, `cmd/multiharness/integration_test.go` |
| Implementation | Execute the supplied plan, capture/resume the right session, retain independently observed changes | `internal/adapter/agent/sessionexec/implementer_test.go`, `internal/workflow/evidence_integration_test.go` |
| Review and repair | Independent review gets current evidence; blocking findings loop back; limits and failed checks never become approval | `internal/workflow/service_repair_test.go`, `internal/adapter/agent/schemaexec/reviewer_test.go`, `internal/workflow/service_failure_test.go` |
| Context / compaction boundary | Repairs work without earlier harness history; original intent and latest evidence survive; a context summary is not completion | `internal/adapter/agent/sessionexec/context_test.go` |
| Provider errors | Billing/auth/access/unknown errors stop; read-only retries are bounded; mutations never auto-replay; billing fallback requires consent | `cmd/multiharness/provider_integration_test.go`, `internal/adapter/agent/provider`, `internal/workflow/provider_policy_test.go`, `internal/workflow/billing_fallback_test.go` |

Supporting tests remain where they protect these behaviors: strict response
parsing, cancellation of child processes, repository preservation, safe provider
logging, and permission/consent boundaries. We removed the duplicate Cucumber
runner, feature files and its dependencies, redundant workflow-through-CLI mocks,
constructor/interface tests, generic JSON round trips, and detailed presentation
matrices. Public behavior tests take priority over private helper assertions or
raising a coverage percentage.

## Run the tests

```sh
make fmt          # Apply standard Go indentation and alignment
make test         # All focused offline tests and supporting safeguards
make integration  # 10 workflow + 12 provider-failure scenarios
make check        # Tests, race detection, vet, build, formatting, modules, fuzz
make coverage     # Statement profile and .coverage/index.html
```

There is no Gherkin engine, step registry or separate acceptance framework;
`make integration` replaces the obsolete `make acceptance` target. All targets above
force live-agent opt-ins off. Fixtures use temporary repositories and local
subprocesses speaking the actual adapter protocols; they never call a model or
install software. `go test ./...` runs the same tests, with live tests skipped
unless their explicit environment opt-ins are set.

The workflow service tests own orchestration rules. Adapter tests own protocol,
permission and session contracts. Integration tests verify they are connected
correctly through the production composition; they do not repeat every classifier
input or configuration permutation. See [coverage.md](coverage.md) for current
measurements and the effect of this cleanup.

## Context compaction boundary

Multiharness does not implement token counting, summarization, a context-window
policy or a compaction engine. Codex planning/review start fresh invocations;
OpenCode implementation may resume its own session. Provider-managed conversation
history is outside the workflow's state.

The offline context test simulates the consequence of discarded history: the
repair process receives only its current stdin payload. It verifies the original
task, plan, latest implementation, validation, blocking findings, repository
baseline/diff and protected files are present. It exercises both a resumed session
and no available session. Another test rejects a narrative context summary in
place of the required structured implementation result.

These tests establish self-contained context handoff. They do **not** execute or
prove native Codex/OpenCode compaction. Go context cancellation/deadline tests are
separate lifecycle checks, not LLM context-compaction tests. Native compaction
requires an independently designed, opted-in live compatibility test; none was run
or added by this test cleanup.

## Live opt-in

Live tests are separate from the offline suite and may consume paid provider
usage. They require installed/authenticated CLIs, an explicit OpenCode model and
an environment without unsafe inherited `GIT_*` overrides. They reject CI.

```sh
MULTIHARNESS_SMOKE=1 \
MULTIHARNESS_SMOKE_MODEL='your-provider/your-model' \
go test -count=1 -timeout 45m -v ./cmd/multiharness \
  -run '^(TestSmokeWorkflow|TestSmokeAgentCancellation)$'
```

This runs immediate approval, a repair loop with a deliberately injected source
fault, and timeout/cancellation probes in disposable repositories. Responses and
validation must be real. Each workflow allows at most eight agent invocations,
zero automatic retries and twenty minutes. These are not monetary caps. The
process-timeout probe can expire during CLI startup and does not prove model
completion.

`MULTIHARNESS_SMOKE_CONFIG` selects an explicit v1 configuration file;
`MULTIHARNESS_SMOKE_MODEL` overrides the implementation model;
`MULTIHARNESS_SMOKE_STAGE_TIMEOUT` sets each agent timeout (default five minutes).
No automatic credential changes, permission elevation or provider switching occur.

### Live billing handoffs

```sh
MULTIHARNESS_SMOKE=1 MULTIHARNESS_SMOKE_FALLBACK=1 \
MULTIHARNESS_SMOKE_MODEL='your-primary-provider/your-model' \
MULTIHARNESS_SMOKE_FALLBACK_MODEL='your-alternate-provider/your-model' \
go test -count=1 -timeout 90m -v ./cmd/multiharness \
  -run '^TestSmokeBillingFallback$'
```

The extra opt-in authorizes one scripted confirmation for each expected fallback
route. Each of four disposable workflows injects a billing failure at one stage;
all other model responses must be real. It never exhausts an account deliberately.
Repeated or unexpected consent requests fail. The alternate OpenCode model applies
to planning/review; Codex implementation uses `fallback.codex_implementer` settings.

## One simple live calculator test

```sh
MULTIHARNESS_SMOKE=1 MULTIHARNESS_SMOKE_MODEL='your-provider/your-model' \
go test -count=1 -timeout 20m -v ./cmd/multiharness -run '^TestSmokeCalculator$'
```

This opt-in demo makes at most three real calls: Codex plans, OpenCode returns
calculator code as text, and Codex reviews it. It does not save or execute the app,
run browser tests or replace the production workflow checks. A rejected review
fails the test. Agent CLIs may retain their own logs/session files.

## Release evidence

No live model, installer or remote CI calls were made for this cleanup. Historical
Codex startup timeout/cancellation probes passed; authenticated end-to-end approval,
OpenCode lifecycle checks and alternate-role handoffs remain release gates. A
skipped smoke test is not a pass. Record actual CLI versions, models, permissions
and outcomes when running them. See [release-readiness.md](release-readiness.md).
