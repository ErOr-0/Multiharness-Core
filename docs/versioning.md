# Versioning

Application releases and wire contracts have independent versions. The repository
is pre-release until the Phase 9 completion gate passes; this document does not
declare or publish a release.

| Boundary | Current version | Authority |
| --- | --- | --- |
| JSON configuration | integer `version: 1` | `internal/config` and `examples/multiharness.json` |
| Planner response (either agent) | string `schema_version: "2"` | `internal/adapter/agent/structured/schemas/plan.v2.json` |
| Reviewer response (either agent) | string `schema_version: "1"` | `internal/adapter/agent/structured/schemas/review.v1.json` |
| Implementation/repair response (either agent) | string `schema_version: "1"` | `internal/adapter/agent/structured/implementation.go` |
| CLI result envelope | string `schema_version: "1"` | `internal/transport/cli.Result` |
| JSONL lifecycle log | integer `version: 1` | CLI presentation/logging contract |

Planner v2 adds the required `implement`/`answer` decision. Planner v1 is not
accepted. Coding plans have steps and acceptance criteria; answer plans have an
answer and no coding steps. Reviewer approval cannot coexist with blocking
findings. OpenCode output is JSONL events, with a versioned JSON object in the
last text event and a consistent session ID; step-finish alone is not completion.

OpenCode final responses may be bare JSON or JSON inside one complete Markdown
fence labelled `json` (or unlabelled). The adapter removes only that outer wrapper
before strict decoding, preserving the JSON contents. Extra commentary, multiple
or incomplete fences and other language labels are not accepted. This handles
model formatting without retrying a call or relaxing any schema/domain checks;
the wire versions and Codex parsing are unchanged.

Agent adapters reject unsupported schema versions, unknown response fields,
missing/null required fields, malformed/trailing documents and invalid domain
invariants. Agent responses also reject duplicate keys (including escaped duplicate
spellings), noncanonical key casing, invalid UTF-8 bytes and excessive nesting.
Keys must match the published lower-case schemas exactly. This fixes permissive
JSON decoding without changing the wire versions; previously accepted ambiguous
or schema-invalid responses now fail. Configuration requires lowercase property
names, rejects duplicate keys (including case-only variants) and any null values;
environment-variable map keys retain their casing. The workflow/store packages contain no vendor schemas
or runtime framework. Internal Go request structures are implementation contracts,
not a separately supported public HTTP/persistence API.

Workspace readiness and lease acquisition now share one intake operation. Runtime
checkout/access failures consistently use `workspace_error`; structural task
validation continues to use `invalid_input`. No terminal status or wire shape changed.

The CLI envelope adds `schema_version`, `task_id`, and `run_id` alongside existing
TaskOutput fields. IDs are opaque and newly generated for each invocation;
all repairs within it share the same IDs. Help is text, not a result envelope.
Cancelled/failed runs may have only partial evidence. Consumers must inspect
`status` and exit code, not just the presence of `implementation` or a log event.
`result_ready` is emitted before stdout is written; it is not proof of delivery.

The current pre-release adds optional `execution` configuration, defaulting to
64 agent launches and zero retries. Results add `agent_invocations` and optional
`failure.provider` (`kind`, `attempts`, optional `retry_after_millis`); the existing
`failed` status can use `invocation_limit_reached`. Retry logs add
`agent_retry_scheduled` with allowlisted provider kind and attempt counters.
Older v1 configuration without `execution` still loads with safe defaults; an
unusually long workflow can now stop at the launch limit. No agent response schema
or terminal status changed. `max_cost_microusd` is only a fail-closed capability
guard: zero means no dollar cap, and all other values are rejected at startup.

## Compatibility rules

Optional v1 `planner_harness` and `opencode_planner` settings select the primary
planning/answer harness. Older configurations retain Codex planning. OpenCode
uses the same planner v2 response contract; no terminal status or response schema
changes. Its billing fallback offers the configured Codex planner only after the
existing explicit consent. The internal state/mediator/strategy scaffolding and
unconnected local-LLM placeholder were removed; these were internal Go types,
not supported CLI, wire or provider integrations.

Optional v1 `color` and `progress` settings default to `auto`; old configurations
still load. Interactive text presentation changes to readable coloured/live
output; redirected text keeps its existing default format. `progress=plain`
selects readable non-animated lines and `off` suppresses progress. JSON stdout
contracts are unchanged. JSONL v1 gains optional allowlisted `agent`/`activity`
fields on `code=agent_activity` notices (sequence zero) and
`retry_delay_millis` on ordered retry events. Activity is coalesced rather than a
lossless event log; consumers must not infer completion from it. Colour settings
never affect JSON. No agent-response schema or approval/consent policy changed.
`result_output_failed` is an additional error notice when stdout delivery fails
after the outcome is ready; successful delivery still requires the process exit
code and complete JSON result, not `result_ready` alone.

Default Codex runtime discovery is now automatic; explicit executable paths or
custom names remain pinned. It selects an installed compatible release before
execution, not a different model or provider. JSONL v1 logs add optional
`runtime_version` on `code=codex_runtime_selected` notices. These presentation
notices have sequence zero and do not advance the workflow event sequence.
No result, agent-response or configuration schema changed.

The confirmed-fallback extension keeps schema versions unchanged. Optional
`fallback` configuration defaults to terminal-only prompting, and optional result
`agent_switches` records the stage, source, destination, model description and
write scope approved by the user. `agent_switched` is a new lifecycle event.
Old configs load with these defaults; non-interactive behavior is unchanged.
Set `fallback.mode` to `disabled` to retain stop-only behavior even in a terminal.
Human consent questions are an exception to pure JSONL stderr in interactive mode.

- Agent response schemas use exact versions. Breaking shape/meaning changes
  require a new version, schema/parser changes, prompt changes and round-trip
  tests. Do not reinterpret an old version or silently accept a newer one.
- Configuration stays on v1 for optional fields with safe defaults (such as
  `log_format`). Removing/renaming fields, changing units or permission defaults,
  or changing interpretation requires a version migration. Older binaries may
  reject newly added config fields; pin the binary and configuration together.
- Result/log consumers should reject unsupported versions and tolerate new
  optional fields within their supported version. Breaking changes or altered
  terminal-status semantics require a new envelope version. New statuses require
  explicit consumer handling; never default an unknown status to success.
- Tag application releases using semantic versioning. Before 1.0, incompatible
  application changes require a minor-version bump; after 1.0 they require a major
  bump. Compatible features are minor and compatible fixes are patch releases.
  Pin external CLI versions and record the smoke-tested models separately.
- Add migration notes and compatibility tests in the same change. Preserve old
  fixtures where compatibility is promised. No silent downgrade/model fallback.
