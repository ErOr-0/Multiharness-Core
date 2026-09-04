# Trust and permissions

## What the coordinator enforces

The workspace adapter captures a baseline before agents run. It derives changed
files/diffs itself, protects pre-existing dirty paths, and detects index/HEAD
changes. Read-only stages (planning, validation, review) must not alter captured
state. Validation executes configured argument arrays directly, with bounded
output and timeouts. Malformed responses and incomplete evidence fail closed.

An implementation summary is an untrusted claim, not proof that files changed
or tests passed. The independent reviewer gets the plan, actual repository
evidence, deterministic validation, and implementation claims. Blocking feedback
goes to the same OpenCode session when available, with the full repair context.

These safeguards are detection and attribution, not a filesystem sandbox or a
transaction. Detection can happen **after** an agent has changed something.
No automatic reset, checkout, stash, rollback, commit, or push is performed.

## What the operator must control

| Boundary | Operator responsibility |
| --- | --- |
| Codex planner/reviewer | App configuration requires read-only sandbox; review installed CLI settings, hooks, plugins, network and provider policy |
| OpenCode implementer | Select the provider/model and audit agent configuration, allowed tools, plugins and permissions |
| Validation commands | Trust the executable and script contents, arguments and inherited environment; commands are not sandboxed by Multiharness |
| Workspace | Use a disposable checkout or external OS/container sandbox for untrusted tasks; keep secrets out of agent-readable locations |
| Provider | Ensure task/repository content may be transmitted under your provider and account policies |

`reject_on_prompt` retains OpenCode's non-interactive rejection of permissions
that require asking. It does **not** deny permissions already allowed by local
configuration. `auto_approve` explicitly adds `--auto`, allowing permissions that
would otherwise prompt while retaining configured deny rules. Do not enable it
to work around a failure without reviewing the permission boundary.

Agent CLIs use their own existing logins and configuration. Multiharness does not
change global settings, provision credentials, select accounts, or enable sharing.
CLIs may independently retain sessions, logs and telemetry. In particular,
OpenCode session persistence is used for in-run repairs; deleting a disposable
checkout does not delete the CLI's own session history.

The process adapter inherits the environment. Prefer minimal, dedicated runtime
environments. Never put secrets into CLI flags, task text, versioned config, or
validation command arguments unless you intend the invoked agent/process to
receive them. `--task-file` avoids argv exposure, not provider visibility.

## Evidence limitations

Billing fallback is a separate consent boundary, not automatic permission
escalation. The question identifies the destination model, usage/data transfer,
and write scope. Only an explicit terminal `yes` enables the alternate role.
Codex fallback implementation is restricted to `workspace-write`; OpenCode fallback
planning/review use fresh deny-by-default agents with source-reading tools and
external plugins disabled. Managed OpenCode settings remain an operator-controlled
override, not something the coordinator can promise to bypass. Session identifiers
never cross agent providers. Do not share stdin between concurrent interactive runs.

- Locks exclude cooperating workflows sharing a Git common directory, not other
  editors or arbitrary processes. Avoid concurrent human edits during a run.
- Pre-existing changed files are protected at whole-file granularity, not hunks.
- Snapshots include tracked and non-ignored untracked files; ignored files and
  side effects outside the checkout are not attributed or restored.
- Symlink targets are captured without reading through them. This is not a
  restriction on what a separately invoked agent or validation command can read.
- Unsupported layouts, special files, invalid paths, and snapshot/diff limits
  stop execution rather than silently weakening the evidence boundary.
- A passing configured check list can miss defects. An empty check list means
  no deterministic validation ran. Review approval is not a universal guarantee.

## Logging and sensitive artifacts

Lifecycle logging permits only known event/stage/status/error-code values,
numeric counters, timestamps and randomly generated task/run IDs. Unknown string
metadata is replaced with `[redacted]`; task content, paths, diffs, session IDs,
provider output, environment values and raw errors are excluded entirely.
This allowlist avoids depending on incomplete credential-pattern matching.

Live progress follows the same rule: the optional activity observer reads bounded
stdout JSONL metadata, never stderr contents or reasoning/message text. It emits
only fixed agent/activity identifiers. Unknown or oversized telemetry is ignored
for display, without weakening authoritative provider-failure or response parsing.
A one-slot latest-wins display mailbox avoids unbounded traffic or blocking child
output on UI writes. Telemetry is lossy and untrusted, never completion evidence.
Terminal colours/control sequences are application-owned, not provider-supplied.
The elapsed timer and provider step completion do not establish successful work.
Billing prompts pause rendering; presentation cannot authorize a provider switch.

Structured agent decisions reject duplicate keys at every nesting level, including
escaped spellings, and require exact lower-case schema keys and valid UTF-8 bytes.
A repeated `approved` or `blocking` field cannot silently overwrite a prior value.
JSON nesting is bounded before typed decoding; unknown-key/type/syntax decoding
errors do not echo the offending input. This applies to the final structured
response, not arbitrary provider event formats or free-form result evidence.

Both text logs and JSONL logs use the same policy. Writer error messages are not
echoed. Log-writer failures cancel the run and cannot yield a successful result.
These are operational logs, not a tamper-proof audit trail.

Recognized provider failures are normalized to safe categories/operator messages;
raw provider error bodies are not copied into those public diagnostics. This does
not sanitize other evidence: full JSON results are **not generally redacted**.
Summaries, diffs, validation output, file paths and answers can contain secrets. Altering them
would corrupt evidence. Protect result files and any recovery artifacts with
appropriate permissions, retention and access controls. Redirect output outside
the target checkout to avoid self-generated concurrent changes.

## Build and dependency gate

`make security` runs pinned `govulncheck` against production and test code. CI uses
read-only repository permissions, immutable action revisions, no checkout credential
persistence, and no provider credentials. The Go minimum is 1.26.6 and `x/sys` is
0.44.0, addressing advisories reported against the prior toolchain/dependency
(including [GO-2026-6218](https://pkg.go.dev/vuln/GO-2026-6218) and
[GO-2026-5024](https://pkg.go.dev/vuln/GO-2026-5024)). The earlier scan found no
reachable vulnerable calls; upgrading also removes those package/module warnings.
A clean vulnerability scan is a point-in-time check, not a security certification.
