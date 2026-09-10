# Multiharness Core

**`magent` coordinates coding tasks across Codex, OpenCode and Claude Code.**
Choose a folder and a provider/model for each role. The workflow plans the task,
hands it to implementation, runs configured checks, reviews the result, and sends
blocking findings back for repair. Questions can be answered directly by the planner.

Workspace execution requires no Git repository or Git executable. Multiharness
works with the folder you select, backs up included files before edits, and checks
changes independently of agent summaries.

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
- `/login codex` signs in using the provider's browser/device flow.
- `/login opencode` configures an OpenCode account. Skip it for an all-Codex team.
- `/config` opens a numbered menu: **1** changes your project folder, **2** changes
  your agent team. Both save automatically. Advanced `/set` changes still use `/save`.
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

On first run, choose a folder and configure **planning, implementation and review
independently**. Each role supports Codex, OpenCode or Claude Code, with its own
model and reasoning effort or variant. Repair uses the implementation role.

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

Or ask: `Explain how authentication works in this folder.` Each new task is
independent. Repairs receive the original task, plan, changes, validation evidence
and review findings, even when provider history is unavailable.

Progress stays compact by default: a dedicated section shows the active stage,
provider, elapsed time and an animated indicator. Command output stays collapsed.
To see the transcript on subsequent tasks, use `/set progress expanded` and `/save`.

| Command | Purpose |
| --- | --- |
| `/workspace` | Choose a folder inside the shared mount |
| `/config` | Configure the folder or the three agent roles |
| `/settings` | Show current settings |
| `/options` | List available settings |
| `/set max-repair-attempts 3` | Allow up to three repair attempts |
| `/set progress auto` | Restore compact progress |
| `/set color never` | Disable colors |
| `/load /path/to/config.json` | Load a configuration |
| `/save` | Save advanced `/set` changes |
| `/help` | Show help |
| `/quit` | Exit while retaining files and settings |

### Configure validation

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
  --planner-harness codex --planner-model gpt-5.6-sol --planner-reasoning medium \
  --implementer-harness codex --implementer-model gpt-5.6-sol --implementer-reasoning medium \
  --reviewer-harness codex --reviewer-model gpt-5.6-sol --reviewer-reasoning high
```

Use `--task-file` for a UTF-8 task file, `--config` for an explicit version-1 JSON
configuration, and `--help` for all options. No configuration is discovered from
the target folder automatically. Examples are in [examples](examples).

Precedence is defaults → selected config file → `MULTIHARNESS_*` environment →
explicit flags. For example, `--planner-model` maps to
`MULTIHARNESS_PLANNER_MODEL`. Empty values and zero repair attempts are real
settings. Models, commands, efforts/variants, timeouts and retry limits are
configurable. Unsupported or ambiguous settings fail before a task starts.

The default team is Codex planning/review (`gpt-5.6-sol`, `xhigh`) and OpenCode
implementation. The wizard lets you change each role and its reasoning separately.
Plain CLI runs emit a structured JSON result to stdout and progress to stderr;
`--log-format jsonl` uses structured progress metadata. Full results can contain
source code and validation output; handle them as private project data.

| Result | Meaning | Exit |
| --- | --- | --- |
| `approved` | Review approved and configured checks passed | 0 |
| `answered` | Planner answered without implementation | 0 |
| `failed` | An error stopped the task | 1 |
| `repair_limit_reached` | Blocking findings remain after allowed repairs | 3 |
| `cancelled` | Interrupted or timed out | 130 |

Usage/configuration errors exit 2. Reaching a limit never means success.
`repair_attempts` counts actual repair invocations, including failed attempts.

## Folder safety and recovery

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

The plain-Go `workflow.Service.Run` coordinates typed stage contracts and ordered
events. The workflow depends on its own ports and `internal/store`; adapters own
CLI protocols, processes, folder inspection, validation and presentation.
`structured.Agent` shares role validation, prompts, schemas and result parsing.
`schemaexec` handles Codex/Claude responses; `sessionexec` handles OpenCode events
and verified session reuse. New providers supply protocol translation and
composition wiring rather than copying role implementations.

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
