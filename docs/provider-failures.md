# Provider failures and commercial-use boundaries

Provider billing failures must not become success, indefinite waits, or automatic
replays of partial implementation work. Both agent adapters use the same bounded
error-stream monitor and return provider-neutral contracts to the workflow.

## Classification and reporting

| `failure.provider.kind` | Meaning | Automatic retry |
| --- | --- | --- |
| `billing_exhausted` | Explicit exhausted credits, spending/usage limit, or payment requirement | Never |
| `authentication_failed` | Invalid/missing authentication | Never |
| `access_denied` | Model/account/project access failure | Never |
| `rate_limited` | Explicit transient request/token rate limit | Read-only stages, opt-in |
| `overloaded` | Service overload/unavailability | Read-only stages, opt-in |
| `unknown` | Unclassified provider error, including ambiguous HTTP 429 | Never |

HTTP 429 alone is not enough to authorize a retry: it can represent exhausted
credits rather than a transient limit. Explicit billing indicators override a
rate-limit label. The mapping follows the distinction in the
[OpenAI error-code documentation](https://developers.openai.com/api/docs/guides/error-codes).
Adapters also accept known OpenCode error envelopes without coupling the workflow
to provider-specific fields. Unknown providers fail closed instead of guessing.

The final status is `failed`, code `agent_error`, with stage, safe operator
message, provider kind, actual attempt count, and optional `retry_after_millis`.
`agent_invocations` counts launches across the whole run. Raw provider messages,
headers, response bodies and keys are excluded from normalized provider errors
and lifecycle logs. Full repository/validation/agent-result evidence can still
contain sensitive data and must be handled securely.

Codex runs with JSON events and its schema-constrained final-output file. OpenCode
uses JSONL events. Explicit provider-error events trigger private child-context
cancellation, even before a session ID exists or if the process would exit zero.
Known `Error:` stderr lines are also observed; nonzero exits have a conservative
stderr fallback. The process adapter terminates the supported Unix process group.
User cancellation/deadline remains distinct from provider-triggered cancellation.
No downstream stage runs after an unhandled terminal agent error. Billing alone
can invoke the human-confirmed role-switch path described below.
Available independent repository evidence is retained, including partial changes.
Duplicate keys in JSON error payloads or event envelopes are rejected before
they can overwrite failure fields. Ambiguous envelopes produce a non-retryable
`unknown` provider failure; JSON examples inside ordinary text remain opaque.

## Retry and launch policy

- Default: zero retries and 64 agent launches per run. Configure retries from
  0–10 and launches from 1–10,000; these are separate from the repair-round limit.
- Only an explicitly transient **planning or review** failure can be retried.
  Repeated requests can still consume additional provider usage.
- Implementation and repair are never automatically replayed. An error cannot
  prove that filesystem, network, or external side effects did not occur.
- Repository evidence must be unchanged before and after the wait. Concurrent
  edits or an ostensibly read-only agent modifying the repository stop retries.
- Exponential backoff uses equal jitter within a configured maximum, default
  1-second initial and 30-second maximum delays. Cancellation interrupts waiting.
- An exposed Retry-After (seconds or HTTP-date) is a minimum wait. If it is invalid,
  overflows, exceeds the configured maximum, or cannot fit the run deadline, the
  workflow stops instead of retrying too early. A header not exposed by the CLI
  cannot be honored by this adapter.
- Every attempt counts toward the launch limit. A failed attempt that consumes
  the remaining budget retains its provider error; a blocked next-stage launch
  reports `invocation_limit_reached`. Neither case means approval.
- There is no automatic provider/model/account switch, credit purchase, credential
  change, or restart when billing is restored.

## Explicit billing handoff

Billing errors are never automatically retried. A human can instead authorize
the alternate agent through the CLI question: OpenCode implementation/repair
switches to Codex, and Codex planning/review switches to OpenCode. When OpenCode
is explicitly selected as the primary planner, its planning fallback is Codex
using the `planner` settings. Consent is
required separately for each role and is recorded in `agent_switches`; the role
then remains on its alternate for the current run. A role switches at most once.
If both agents have exhausted billing, the run stops. The alternate can still use
the same underlying account/provider, so switching CLIs does not guarantee credits
are available. See [CLI configuration](cli.md#confirmed-billing-fallback).

Codex implementation uses a schema-constrained, ephemeral `workspace-write`
invocation with the full plan, current baseline-relative evidence, and repair
feedback when applicable. This follows the documented
[Codex non-interactive permission model](https://learn.chatgpt.com/docs/non-interactive-mode).
OpenCode planning/review use fresh sessions and a uniquely named runtime agent
with all tools denied except read/glob/grep/list; shell/edit/subagent access is not
enabled. `--pure` disables external plugins. Existing inline provider settings are
preserved in the child configuration, without changing global files or credentials.
Permissions follow the documented [OpenCode agent rules](https://opencode.ai/docs/agents/)
and [configuration precedence](https://opencode.ai/docs/config/). Managed settings
can override runtime configuration; operators must audit them. This is not an OS
sandbox. The workflow separately detects changes during read-only stages.

The failed process must return before consent, and repository safeguards run
before and after the question. An alternate receives partial Git evidence and
instructions to inspect completed work rather than blindly replay external side
effects. This is an in-memory continuation, not a transactional rollback or a
durable checkpoint. If evidence is unsafe, consent cannot override that failure.

## What this does not guarantee

The launch counter is not a token or monetary ledger. A single CLI invocation may
issue multiple model requests or internal retries before reporting an error;
subprocess cancellation cannot reverse usage already incurred or guarantee a
remote request stopped. The adapter recognizes supported error envelopes and
bounded diagnostic lines, not every future CLI/provider protocol. Pin CLI versions
and run authenticated compatibility tests before release.

`execution.max_cost_microusd` deliberately rejects **every nonzero value** before
starting an agent. Zero means no monetary cap. This prevents callers from believing
an unsupported budget is enforced. A guaranteed customer dollar cap requires an
authoritative metered gateway/provider enforcement, reservations across concurrent
runs, settlement of actual usage, and tests for retries and partial failures.

There is no durable run checkpoint, cross-run idempotency ledger, automatic resume,
or multi-tenant billing isolation in this local synchronous core. Commercial
deployment requires an explicit design and verification for those service-level
requirements, as well as the pending live-agent compatibility gate. Passing the
deterministic suite is necessary but does not certify the whole commercial product.

## Verification

Go integration tests run the production composition with fixture agent
subprocesses and real Git evidence. Unit tests isolate classification, backoff,
cancellation, limits and consent; a fuzz target checks malformed provider errors
and secret-safe output. Run `make integration`; see [testing.md](testing.md).
