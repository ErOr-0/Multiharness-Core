# Multiharness Core

**`magent` coordinates coding tasks across Codex, OpenCode and Claude Code.**
Choose a folder and one agent. In **direct mode (the default)**, Multiharness
passes your task to that CLI, lets it handle planning/editing/testing, and shows
its final response. Follow-ups reuse the same agent session; `/new` starts fresh.

Use `/set mode team` (or `--mode team`) when you want separate planning,
implementation, deterministic validation, review and bounded repair. The team
workflow also owns folder snapshots and independent change evidence.

Both modes support folders without Git.

## Docker setup

The terminal application uses image `er0r2/multiharness:latest`, container
`multiharness`, and volume `magent-state` for accounts, settings and recovery copies.
It exposes no web port. Docker Desktop must use Linux containers on Windows.

### First run

Install Docker Desktop (Linux containers on Windows), or Docker Engine with the
Compose plugin on native Linux. Install Git on the computer too: Compose uses it
to download the configuration and security files from the public repository.
Use a current Docker Compose with Git remote support (verified with v5.3.1).
No ZIP extraction, host launcher, setup script or manual `.env` file is required.

1. Download the image:

   ```sh
   docker pull er0r2/multiharness
   ```

2. Define the **absolute path to the existing folder containing your projects**
   in the command below, then run it. You can run from any directory. The
   [website](https://multiharness.mdfahimhossen.space/#start) generates the command
   for your path and platform, including correct quoting. Folder paths entered on
   the website stay in your browser. Native Linux users must also follow the
   AppArmor section below if their host uses AppArmor.

   **macOS — Terminal:**

   ```sh
   MULTIHARNESS_WORKSPACE='/path/to/Projects' docker compose -f 'https://github.com/ErOr-0/Multiharness-Core.git#main' create &&
   docker start -ai multiharness
   ```

   **Windows — PowerShell:**

   ```powershell
   $env:MULTIHARNESS_WORKSPACE = 'D:\Projects'
   docker compose -f 'https://github.com/ErOr-0/Multiharness-Core.git#main' create
   if ($LASTEXITCODE -eq 0) { docker start -ai multiharness }
   ```

   **Native Linux without AppArmor — Terminal:**

   ```sh
   MULTIHARNESS_WORKSPACE='/path/to/Projects' MULTIHARNESS_UID="$(id -u)" MULTIHARNESS_GID="$(id -g)" docker compose -f 'https://github.com/ErOr-0/Multiharness-Core.git#main' create &&
   docker start -ai multiharness
   ```

   Docker Compose fetches the maintained configuration and scoped seccomp file,
   then creates the single named container. It keeps all capabilities dropped and
   no-new-privileges enabled. The start command runs only if creation succeeds.
   Docker stores your selected host mount in the container; the app does not need
   a host setup script to remember it.

3. Inside the app, choose a project within the shared folder, then your agents
   and models. Completed settings save automatically. Sign in with `/login codex`
   and, if selected, `/login opencode` or `/login claude`, then type your task.

Docker must receive the host folder at creation time. `/config` can then change
projects within that shared folder; it cannot attach another host folder to an
already-created container. `/cancel` discards incomplete team configuration.

### Everyday use

```sh
docker start -ai multiharness
```

This starts the same stopped container. It does not create another container.
Inside the application:

- `/workspace` selects a folder within the shared tree. Your selection is saved
  automatically and checked again on the next start.
- `/configuration` shows the selected roles, models and current prerequisite status.
- `/setup` walks through missing account sign-ins. Configuration offers this setup
  immediately after selecting agents. Tasks stay blocked until all required checks pass.
- Type `/` to see commands while typing. Use Up/Down to select, Tab or Enter to
  fill the suggestion, then Enter to submit. `/set` also suggests common values.
- `/login codex` signs in using the provider's browser/device flow.
- `/login opencode` configures an OpenCode account. Skip it for an all-Codex team with unused fallbacks disabled.
- `/config` opens a numbered menu: **1** changes your project folder, **2** changes
  your agent (or planner, implementer and reviewer in Team mode), **3** changes
  the selected agent's permissions, and **4** switches between Direct and Team.
  The menu shows the current mode. These choices save automatically; `/cancel`
  keeps the current settings. `/settings` shows current values and `/options`
  lists all controls, including timeouts and progress. Team mode also uses
  validation checks, repair limits and retries. Advanced `/set` changes use `/save`.
- Type a task to begin. No validation checks run unless you configure them.
- `/quit` stops the application, retaining the container, files and settings.

If already running, reconnect with `docker attach multiharness`. Use one attached
interactive client at a time. Docker's Ctrl+P, Ctrl+Q detach keys leave it running;
Ctrl+C cancels active work and exits. Docker Desktop can start/stop the same named
container. To interact, attach from a terminal; no browser window is opened.

Provider sign-in commands run inside this container; they do not create helper
containers. Credentials remain under `/state/<uid>` and are not part of the image.

### Updating the container

Type `/quit`. Run `docker pull er0r2/multiharness`, then repeat your first launch
command with the **same host folder** and Linux policy choice. Compose replaces
only the named container when needed and retains the `magent-state` volume.
Existing app settings and provider logins load from that volume. Use the same UID
on Linux. Never delete `magent-state` or use `down -v` to update.

Use `/config` → **1** to switch projects inside the shared folder. To share a
new host folder, quit and repeat the launch command with its absolute path. This
recreates the same named service with the new mount and existing state. If the
saved project is unavailable, the app asks you to choose one again.

### Linux AppArmor

Docker Desktop users do not need this step. On native Linux, check for
`name=apparmor` in `docker info --format '{{json .SecurityOptions}}'`.
If present, install the named host policy once before creating the container:

```sh
curl --fail --location 'https://raw.githubusercontent.com/ErOr-0/Multiharness-Core/4c3fe37e9346e1d0bcd2511601f3f5bfc2efdeb7/docker/apparmor.profile' --output "$HOME/.multiharness-apparmor.profile" &&
sudo apparmor_parser --replace --skip-read-cache "$HOME/.multiharness-apparmor.profile" &&
sudo install -m 644 "$HOME/.multiharness-apparmor.profile" /etc/apparmor.d/magent-container-v1
```

These commands download a pinned policy file, load it and save it for reboot.
They do not execute a downloaded script. The profile applies only to containers
that explicitly select `magent-container-v1`; other Docker profiles and host
sysctls stay unchanged. An unsupported AppArmor parser fails; do not disable the
filters as a workaround. See [third-party notices](#third-party-notices) for provenance.

Include the Linux override in the first launch command and future updates:

```sh
MULTIHARNESS_WORKSPACE='/path/to/Projects' MULTIHARNESS_UID="$(id -u)" MULTIHARNESS_GID="$(id -g)" docker compose -f 'https://github.com/ErOr-0/Multiharness-Core.git#main' -f 'https://github.com/ErOr-0/Multiharness-Core.git#main:docker/compose.linux.yaml' create &&
docker start -ai multiharness
```

The website includes this override when the AppArmor option is selected.
Everyday start remains `docker start -ai multiharness`.


## Choose your agents and give it a task

On first run, choose a folder and configure **one agent**: harness, model, and
reasoning/variant. Direct mode reuses the saved `implementer` settings. No planner
or reviewer is started. `/settings` shows the active agent and effective deadline.

For independent roles in Docker, use `/config` → **4** → **2** (Team), then
`/config` → **2** to configure each role. Both menus save automatically. Switching
modes starts a new conversation. For the native binary, use `/set mode team`,
then `/config` to configure and save the team. Each role
supports Codex, OpenCode or Claude Code; repair uses the implementation role.

The setup wizard shows numbered reasoning choices for Codex and Claude; type a
number or name. Enter keeps the displayed value. OpenCode models use
`provider/model` with an optional variant. Higher reasoning can take longer.
Use a model your account supports; model availability is checked by the provider.

Sign in to each selected provider using `/login codex`, `/login opencode` or
`/login claude`. Tasks use those accounts and may consume paid usage. Completed
setup saves automatically; `/cancel` discards an unfinished setup.

Type a task, for example:

```text
Add a health-check endpoint and tests for it.
```

Or ask: `Explain how authentication works in this folder.` Direct follow-ups keep their session until `/new` or an agent/workspace/mode change.
Sessions are not saved in personal configuration. Team tasks are independent; repairs receive the original task, plan, changes, validation evidence
and review findings, even when provider history is unavailable.

Progress stays compact by default: a dedicated section shows the active stage,
provider, elapsed time and an animated indicator. Command output stays collapsed.
To see the transcript on subsequent tasks, use `/set progress expanded` and `/save`.

| Command | Purpose |
| --- | --- |
| `/workspace` | Choose a folder inside the shared mount |
| `/config` | Configure agents; Docker menu also includes folder, permissions and Direct/Team mode |
| `/new` | Start a fresh direct conversation |
| `/set mode direct` | Use one agent; `/set mode team` enables the full workflow |
| `/settings` | Show current settings |
| `/permissions [MODE]` | Set and save the selected agent's permissions, retaining the conversation |
| `/options` | List available settings |
| `/set max-repair-attempts 3` | Allow up to three repair attempts |
| `/set progress auto` | Restore compact progress |
| `/set color never` | Disable colors |
| `/load /path/to/config.json` | Load a configuration |
| `/save` | Save advanced `/set` changes |
| `/help` | Show help |
| `/quit` | Exit while retaining files and settings |

### Configure validation (team mode)

Checks are empty by default. That means **no tests ran**, not that tests passed.
For a Go project:

```text
/set validation-checks [{"executable":"go","args":["test","./..."]}]
/save
```

Choose checks appropriate to your project. Commands run after implementation and
after each repair, with configured timeouts and bounded output. Checks must not
modify inspected source files; ignored build artifacts are outside that boundary.

## CLI and configuration

For unattended runs, pass a task and folder directly:

```sh
magent --workdir /path/to/project --task 'Add a health-check endpoint' \
  --implementer-harness codex --implementer-model gpt-5.6-sol --implementer-reasoning medium
```

Use `--task-file` for a UTF-8 task file, `--config` for an explicit version-1 JSON
configuration, and `--help` for all options. No configuration is discovered from
the target folder automatically. Examples are in [examples](examples).

Precedence is defaults → selected config file → `MULTIHARNESS_*` environment →
explicit flags. For example, `--planner-model` maps to
`MULTIHARNESS_PLANNER_MODEL`. Empty values and zero repair attempts are real
settings. Models, commands, efforts/variants, timeouts and retry limits are
configurable. Unsupported or ambiguous settings fail before a task starts.

Direct mode uses the configured implementer. The optional team defaults to Codex planning/review (`gpt-5.6-sol`, `xhigh`) and OpenCode
implementation. The wizard lets you change each role and its reasoning separately.
Plain CLI runs emit a structured JSON result to stdout and progress to stderr;
`--log-format json` uses structured progress metadata. Full results can contain
source code and validation output; handle them as private project data.

| Result | Meaning | Exit |
| --- | --- | --- |
| `responded` | Direct CLI returned text; not independent proof of task completion | 0 |
| `needs_input` | Native CLI reported a permission/input block | 4 |
| `timed_out` | Direct invocation deadline expired; partial output retained | 124 |
| `approved` | Review approved and configured checks passed | 0 |
| `answered` | Planner answered without implementation | 0 |
| `failed` | An error stopped the task | 1 |
| `repair_limit_reached` | Blocking findings remain after allowed repairs | 3 |
| `cancelled` | Interrupted or timed out | 130 |

Usage/configuration errors exit 2. Reaching a limit never means success.
`repair_attempts` counts actual repair invocations, including failed attempts.

## Folder safety and recovery (team mode)

The selected folder is the boundary. Multiharness does not inspect parent
repositories, commits, staging or Git status. It captures included file contents,
modes and symlink targets, then compares each round with that starting state.
Validation and review must leave the captured files unchanged.

Local `.gitignore` and `.magentignore` patterns are parsed directly in Go. Nested
rules and negations apply locally; ignored directories are not traversed. `.git`,
`.hg` and `.svn` metadata are excluded. No global Git configuration or index is
read, so tracked status does not override ignore patterns. Add dependency/build
folders to local ignore files. Changing a rule cannot hide a baseline file.
Symlink targets are recorded without following them outside the selected folder.

Before implementation, a private recovery copy saves included files:

- `--existing-work snapshot` is the default: back up and allow task-scoped edits.
- `--existing-work prompt` asks before modifying existing files; no/EOF stops edits.
- `--existing-work preserve` protects every file present at the start.

All modes require preserving unrelated content. The backup path appears in the
result as `repository.recovery_directory`. It contains `files/` and a
`manifest.json` identifying the starting folder and fingerprint. Copies remain
after successful and failed runs. Default storage is `magent/recovery` beneath
the personal configuration directory, on Docker's persistent state volume.
`--recovery-dir` chooses another directory outside the workspace.

Backups exclude ignored files, VCS metadata and external effects. They are not
automatic rollback or durable workflow resume. After a failure, stop the run,
compare current files with its backup, and retain or restore changes selectively.
A new task starts from the current folder. Do not delete the state volume during
updates. Older containers stored recovery under `/tmp`; deleting those containers
may already have removed their recovery copies.

Folder locks prevent cooperating runs from overlapping; they do not block human
editors or sandbox provider commands. Avoid concurrent edits. Snapshot/diff limits
are unlimited by default and memory/disk use grows with folder size. Configure
`workspace.timeout`, `workspace.max_files`, `workspace.max_file_bytes`,
`workspace.max_snapshot_bytes` and `workspace.max_output_bytes`, or the matching
`--workspace-*` flags. Positive limits fail closed on incomplete evidence.
Legacy version-1 `git` settings and `--git-*` inspection flags are migration aliases;
the old executable setting is unused. New configurations use `workspace`.
Legacy result fields `repository`, `head` and `status` remain compatible, with
`head` and `status` empty.

### Provider errors and permissions

Direct mode uses the native CLI session and configuration, with no forced output
schema, retry, automatic provider switch, or review loop. `/permissions` opens a
menu for the selected agent; Docker `/config` option **3** opens the same menu.
Choices save automatically and apply to the next invocation, including a resumed
conversation. `/settings` shows the current choice. Switching agents resets their
permissions and other provider-specific settings to the new agent's defaults.

| Selected agent | Direct-mode choices | Native behavior |
| --- | --- | --- |
| Codex | `workspace` (default), `read-only`, `full` | `sandbox_mode` is workspace-write, read-only, or danger-full-access; `approval_policy` remains never |
| Claude | `native` (default), `edits`, `auto`, `full` | `--permission-mode` is dontAsk, acceptEdits, auto, or bypassPermissions |
| OpenCode | `native` (default), `auto` | Native noninteractive rules, or `--auto` to approve requests |

Use `/permissions MODE` for a direct selection. `native` also restores Codex's
workspace-write default. Codex `full` disables its sandbox and allows access
outside the project. Claude `full` selects its native bypass mode; native deny
rules and managed restrictions still apply. Claude `auto` uses its own approval
classifier and requires a supported account/model; it is not unconditional
approval ([Claude permission modes](https://code.claude.com/docs/en/permission-modes)).
OpenCode `auto` approves requests including external paths, while preserving
explicit deny rules. Each menu describes that provider's scope before selection.
No mode changes automatically in response to a denial. Claude keeps the existing
Read/Glob/Grep/Edit/Write allowlist; other tools follow the selected native mode.
The CLI may return a question: `responded` means a response was received, not that
all requested work was completed. Provider output is never reclassified by prose
heuristics. The smaller of `--timeout` and `--implementer-timeout` applies; a
provider-side deadline is reported separately. Partial responses and session IDs
are retained, and edits are never automatically rolled back or retried.

The following additional rules describe **team mode**:


Read-only planning/review enforce provider permissions. Planning precedes the
folder snapshot, so its read-only behavior relies on the provider boundary.
Codex writes use workspace-write; OpenCode auto-approval is an explicit opt-in.
Claude uses fresh print-mode calls, read tools for planning/review and Edit/Write
for implementation; shell, MCP, subagents and hooks are unavailable. Configured
validation runs separately. Provider permissions are not a universal OS sandbox.

Recognized billing, authentication, rate-limit and availability errors stop with
safe messages. Read-only retries are explicit and bounded, with zero as the
default. Implementation and repair are never automatically replayed. Supported
billing fallbacks require explicit terminal consent; there is no silent account
switch. Native Claude and primary OpenCode review have no automatic fallback.

Missing default agent CLIs can offer a confirmed installation when trusted npm is
available. Refusal, EOF and noninteractive input cannot authorize installation.
A failed/cancelled task may leave partial changes. Approval is not a guarantee
that every defect has been found.

## Development and verification

The plain-Go `delegation.Service.Run` handles one native CLI turn;
`workflow.Service.Run` coordinates typed stage contracts and ordered
events. The workflow depends on its own ports and `internal/store`; adapters own
CLI protocols, processes, folder inspection, validation and presentation.
`structured.Agent` shares role validation, prompts, schemas and result parsing.
`schemaexec` handles Codex/Claude responses; `sessionexec` handles OpenCode events
and verified session reuse. New providers supply protocol translation and
composition wiring rather than copying role implementations.

The shared response reader accepts the supported `schema_version` as a string
or integer (`"1"`/`1`, or `"2"`/`2` for plans); native output schemas continue to
request canonical strings. This tolerance is limited to version metadata.
Approval flags, findings, required fields, duplicate keys, unsupported versions
and native completion evidence remain strictly validated for every agent.
Provider request failures retain a bounded category and, when recognized, the
unsupported parameter name in the result and `/diagnostics`. Raw provider text
and credentials are not saved. These errors never silently switch the user's
model, change tool permissions or replay a task; upstream service compatibility
still requires a working native CLI/provider combination.

Keep dependencies directed toward the workflow core, make retries/side effects
explicit, preserve unrelated work, and add focused behavior tests for changes.
Run formatting before static checks. Do not duplicate workflow suites or add
coverage-only tests. Native provider history/compaction is outside this app;
offline tests verify complete context handoff, not native compaction.

```sh
make fmt
make check                 # static, offline tests, race tests, provider fuzzing
make integration           # production composition with fixture providers
make security lint-workflows
make build-dev             # dist/multiharness-dev; no host command replaced
docker build -t multiharness:check .
python3 scripts/test-docker.py multiharness:check
```

Executable Gherkin acceptance tests use Behave and launch the built application
as a subprocess. Install the test-only dependencies in a virtual environment:

```sh
python3 -m venv .venv-bdd
. .venv-bdd/bin/activate # Windows PowerShell: .venv-bdd/Scripts/Activate.ps1
python -m pip install -r tests/acceptance/requirements.txt
python -m behave --junit --junit-directory reports/acceptance
python -m behave --tags=@packaged -D image=multiharness:check
python -m behave --tags=@live -D live_config=/absolute/path/to/your/config.json
python -m behave --tags=@live_team -D live_config=/absolute/path/to/your/config.json
python -m behave --tags=@live_permission -D live_config=/absolute/path/to/opencode-config.json
python -m behave --tags=@live_permissions_ui -D live_config=/absolute/path/to/opencode-config.json
python -m behave --tags=@live_codex_permissions_ui -D live_config=/absolute/path/to/codex-config.json
python -m behave --tags=@live_genkit -D live_config=/absolute/path/to/your/config.json
```

The default 49 contract scenarios use a clearly identified executable provider
fixture to verify actual arguments, stdin, file edits, native session handoff,
permission denial, malformed output, exit codes and process termination. Team
cases exercise Codex, OpenCode and Claude protocols in every role, actual file
validation, supported version representations, rejected unsafe review results,
and unsupported-parameter failures without model switching or replay. The
packaged scenarios exercise the Docker entrypoint and a real terminal in
disposable mounts. One invokes the actual bundled Claude CLI and its permission
engine against a local simulated Anthropic model, with external networking
disabled. It verifies denied shell writes, accepted file operations, full-mode
outside writes, revocation and native session continuity. These checks are not
evidence of authenticated Claude model behavior or auto-classifier availability.
The separate live
scenario invokes the configured native CLI, requires generated code to pass
independent arithmetic checks, and verifies a random token survives a follow-up
without appearing in project files. It fails if no explicit account configuration
is supplied; it never silently skips or falls back to fixtures. Use `-D binary=...`
to test an existing release executable. Reports distinguish selected scenarios
from unrun live checks. CI runs the contract and packaged checks without accounts.

The separate `@live_permission` scenario reproduces an actual OpenCode denied
read outside the selected project, then continues the same native conversation
with an in-project edit. OpenCode rejects permission prompts in non-interactive
mode. Multiharness reports `needs_input` with the blocked tool/path; it preserves
the conversation. Use `/permissions` (or `/config` → **3**) to choose native rules
or auto-approve permission requests, then retry the task. `/permissions auto`
passes OpenCode's `--auto` on subsequent invocations, including resumed sessions.
This approves all permission requests, including paths outside the project;
explicit deny rules in OpenCode still apply. `/permissions native` restores native
rules and non-interactive rejection of approval requests. Both choices save
automatically. For individual path/tool rules, configure OpenCode's native
permission settings. A rejected read can stop the native run even
when its process exits zero and its last step reports `tool-calls`.
Team mode also reports this as `needs_input` (exit 4), identifies the denied
tool/path, and stops before validation or review. Use `/permissions` to change
implementation access, then resubmit the original task. Team starts a new
workflow and inspects current files; it does not automatically replay a blocked
implementation. Planner/reviewer permissions remain read-only and are not
changed by that menu. An unfinished stream without a denial is a failure, even
if an earlier progress message contains valid JSON.

The Team contract scenario replays sanitized native module-cache denials through
real application/fixture processes, checks partial-file preservation, and then
checks actual validation and review after permission changes. Team contracts
require Linux or macOS; on Windows, run them inside Docker.
The opt-in Linux `@live_team_permissions` scenario uses the real authenticated
OpenCode implementer with fixture planning/review. It changes permissions through
the actual terminal and independently verifies the resulting file contents;
it is not a full live multi-agent test. User configuration is checked unchanged.
The separate `@live_team` scenario uses the configured native agent in all three
roles without fixtures. It repairs a synthetic arithmetic function, runs an
independent validation process and requires a successful independent review.
It preserves agent/model preferences and checks saved configuration is unchanged.
Each live run proves only the selected provider/model combination at that time.

The Linux `@live_permissions_ui` scenario drives a real terminal with the selected
OpenCode account: deny an outside-file read, enable permissions in the menu, retry
and verify the actual file contents, then revoke permission and verify another
read is denied. It checks saved settings, native arguments and the same session
across all three invocations. App settings and test permission overrides are
isolated; the account's saved configuration is not changed.
The Linux `@live_codex_permissions_ui` scenario runs the authenticated Codex CLI
through read-only, workspace-write, full-access and read-only again, checking real
filesystem outcomes, saved settings and the same native session throughout.
The opt-in `@live_genkit` scenario fetches current public documentation through
the agent and requires a real Genkit Go scaffold with an executable test to pass
independent `go test` and `go build` checks. It needs network access and consumes
native provider usage, but no model API key for the generated deterministic flow.

Live tests are separate and may consume paid usage. They require authenticated
providers and explicit opt-in, run in disposable folders and refuse CI:

```sh
MULTIHARNESS_SMOKE=1 MULTIHARNESS_SMOKE_CONFIG=examples/codex-team.json \
go test -count=1 -timeout 45m -v ./cmd/multiharness \
  -run '^(TestSmokeWorkflow|TestSmokePlainFolderAnswer|TestSmokeAgentCancellation)$'
```

`MULTIHARNESS_SMOKE_MODEL` overrides the implementation model;
`MULTIHARNESS_SMOKE_STAGE_TIMEOUT` controls stage timeout. Billing-handoff tests
also require `MULTIHARNESS_SMOKE_FALLBACK=1` and an explicitly selected fallback
model. Offline passing results do not imply authenticated live-provider success.

### Website and releases

The product website is a static React/Vite app in `web`. Run `npm ci`,
`npm run dev`, `npm run test:content`, `npm run test:e2e` and `npm run build`
from that directory. The build creates `web/dist`; deploy that directory to the
static host. The workflow animation is a simulation and makes no model calls.
Builds package the maintained Compose/security files and this README into an
optional configuration ZIP; the normal setup uses Docker Compose directly.

The [Docker workflow](.github/workflows/docker.yml) checks amd64 and arm64.
Pushing source does **not** publish a new Docker Hub image. Its explicit publish
workflow builds/tests versioned images and then updates the multi-platform
`latest` tag. Keep server/site updates and image publication distinct.

## Third-party notices

`seccomp.json` is derived from Moby's default Docker profile at commit
`61eaf32614c7c71b60bd8927d3e6a4ffc8ff1f31`:
https://github.com/moby/profiles/blob/61eaf32614c7c71b60bd8927d3e6a4ffc8ff1f31/seccomp/default.json

The upstream Apache 2.0 license is included as `LICENSE.moby`.

The only behavioral change adds an allow rule for `clone`, `unshare`, `setns`,
`mount`, `umount2`, and `pivot_root`. This lets non-root Codex create its inner
user, mount, and PID namespaces. Docker's other default rules remain; `clone3`
still returns the upstream ENOSYS fallback without CAP_SYS_ADMIN. JSON indentation
is normalized to two spaces.

`apparmor.profile` derives from Moby v28.0.0's
[default AppArmor template](https://github.com/moby/moby/blob/v28.0.0/profiles/apparmor/template.go),
under the same Apache 2.0 license. It retains the default proc/sys denials,
same-profile ptrace rule and runtime signal reception. It replaces `deny mount`
with mount/pivot-root permissions and explicitly allows user namespaces for
AppArmor 4. The fixed name is `magent-container-v1`. This permits Codex/bubblewrap's
inner namespace construction; it does not grant CAP_SYS_ADMIN to the outer
container. `scripts/magent-apparmor.sh` explicitly loads and persists only that
named profile on a Linux AppArmor host. It requires AppArmor 4 or newer and root
for that one host setup operation. Launchers select it when Docker reports
AppArmor, and never install it automatically. Docker Desktop hosts without
AppArmor continue using the existing seccomp policy.

This expands the kernel operations available to container processes. The
launchers compensate by dropping all host capabilities and enabling
`no-new-privileges`; they do not enable privileged mode, mount the Docker socket,
disable host AppArmor, change host sysctls, or disable Codex's read-only/workspace-write enforcement.
The profile is applied to this container only. It is not a replacement for
provider sandbox checks or a guarantee of support on every Docker host.

When updating the profile or pinned Codex runtime, run the offline smoke tests
that demonstrate both permitted workspace edits and denied read-only edits.
Additional host restrictions (for example a stricter administrator policy) may
still prevent sandbox startup. Report those failures rather than automatically
relaxing the host's security policy.

### Verify the optional Jev decision router

The router is disabled by default. When enabled in Team mode, the app uses
`OPENROUTER_API_KEY` or asks for the key in an interactive terminal with input
hidden. An entered key is kept only in memory for the app session, never saved
in configuration or exported to coding agents. Empty input cancels startup;
scripted runs without a key stop with setup instructions. Direct mode does not
use Jev or ask for its key.

To verify its real OpenRouter integration,
configure `OPENROUTER_API_KEY` in your local environment and run `make live-jev`.
This sends two small authenticated requests to `typesafe/jev-1.13` at the
OpenRouter Decisions endpoint: one planning request and one review request.
It consumes OpenRouter usage. No project files or account credentials are sent
as decision context; the test uses fixed synthetic examples.

The test requires real HTTP success and parsed model decisions. Missing keys,
transport failures, invalid responses and heuristic fallbacks fail the test.
Normal `make check` remains offline and does not execute this test. A passing
run verifies the selected Jev model at that time; it does not test the native
coding agents or establish the correctness of every routing judgment.

### Workflow readiness

Interactive startup, configuration changes and task submission check the selected
workflow before starting an agent. Direct mode checks its one agent. Team mode
checks planner, implementer, reviewer and enabled fallbacks. Disable unused
fallbacks with `/set fallback-mode disabled`. Each role shows its own status in
`/configuration`; changing a provider, model or workspace triggers fresh checks.

Codex and Claude use their native login-status commands. OpenCode checks that the
selected `provider/model` is available in its effective configuration, including
provider credentials, environment configuration and providers that need no login.
Choose an explicit OpenCode model so the required provider is unambiguous. Native
checks establish local setup, not remote token validity, model entitlement or
remaining credits. Provider requests may still fail, and no task is automatically
replayed after sign-in. `/login` uses the selected executable and works in native
and Docker sessions; Docker stores agent logins in its persistent state volume.

When Jev is enabled in Team mode, setup requests its missing OpenRouter key using
hidden input and validates it with OpenRouter's unbilled key endpoint. Entered keys
remain session-only. Invalid keys, exhausted key spending limits and unavailable
checks block task startup for OpenRouter. Custom Jev endpoints keep their own
credentials; the screen explicitly reports that remote authentication is unverified. Direct
mode and workflows with Jev disabled do not request an OpenRouter key.

Command suggestions apply only to the task prompt. Setup answers, permission
prompts and hidden API-key input do not use completion or persistent input history.
