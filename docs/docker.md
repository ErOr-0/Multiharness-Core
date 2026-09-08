# Run Multiharness with Docker

The preview image is `er0r2/multiharness-core:preview`, with Linux amd64 and
arm64 variants. Docker selects the architecture. This runs locally with your
own provider accounts; Docker Hub distributes the software only.

Install and start Docker Desktop on Windows/macOS, or Docker Engine on Linux.
Windows uses Linux containers; Docker Desktop commonly still uses WSL 2 behind
the scenes. You can run the launcher from PowerShell without installing Git,
Codex, OpenCode, Go, or Node on Windows. Native Windows `magent.exe` execution
remains unsupported.

## Recommended: Docker configuration, original host files

1. Open [Docker setup on the website](https://multiharness.mdfahimhossen.space/#start).
2. Choose your operating system and enter an existing absolute **parent folder**,
   such as `D:\Projects`. The website prepares the download entirely in your browser.
3. Download and extract `multiharness-docker.zip` outside your project. Keep
   `compose.yaml` and `seccomp.json` together. These are Docker configuration,
   not launcher scripts. The policy is the same scoped sandbox policy used by the
   previous launchers; its notice and license are included.
4. Open PowerShell, Command Prompt or Terminal **in that extracted folder**.
   On Linux, complete the AppArmor and UID instructions below first.
5. Sign in once (choose only the providers you intend to use):

```sh
docker compose run --rm magent setup
```

Then start your session:

```sh
docker compose run --rm magent
```

Docker pulls the image if missing. A folder browser opens before your first task.
Press **Enter** to use the folder currently shown, or navigate first:

```text
cd api                 Open a subfolder (a menu number also opens it)
cd ..                  Go to the parent, within the shared folder
ls                     Refresh the folder list
pwd                    Show the current path
mkdir "New Project"    Create a folder directly on your PC
cd "New Project"       Open it, then press Enter to select it
```

`/cancel` exits the browser without changing the selected workspace. Folders you
explicitly create with `mkdir` remain; cancelling is not a filesystem rollback.
These are built-in directory operations, not a shell. They do not execute arbitrary
commands. To create a nested directory, navigate to its existing parent first.
The menu displays up to 50 folders from a bounded listing; `cd PATH` can open
folders not listed. Git is optional.

Use `/workspace` to browse again, or `/config` for workspace and team settings.
The initial browser is not the task prompt: select a folder before using `/config`.
Use `/save` to keep your team. Each launch asks which workspace to use.

**Windows paths belong in Docker configuration.** For example, share `D:/QNE`
as the bind source in `compose.yaml`; the browser then sees its contents under
`/workspace`. Typing `D:\QNE` inside a Linux container cannot grant a new host
mount. Exit, update the source, and restart to change the shared parent folder.

### Where your files and settings live

| Data | Location |
| --- | --- |
| Original source files | Your host folder, bind-mounted at `/workspace` |
| Credentials and personal configuration | Persistent Docker volume `magent-state` |
| Multiharness, Codex, OpenCode and build tools | Docker image |

For example, `/workspace/api/main.go` is the original `D:\Projects\api\main.go`.
There is no second working checkout or synchronization. Multiharness still takes
bounded safety snapshots to detect changes and protect existing work; those are
verification/recovery evidence, not a second project that you must manage.

A selected subfolder scopes the workflow, but is not an additional Docker mount
boundary: only mount a parent folder you intend the container to access. The
picker checks symlinks and rejects paths outside `/workspace`. Host folders not
mounted cannot be added from inside the application. To change the parent folder,
exit and edit the bind `source` in `compose.yaml`, or download a new configuration.
Missing host paths fail without creating empty folders on your computer.

Docker Desktop's basic Run dialog does not expose all required sandbox options.
Use Compose to configure them without scripts. This remains a terminal application;
there is no browser dashboard and no port to publish. A detached `compose up` is
not the interactive start command.

### Later sessions and updates

Reuse `docker compose run --rm magent`. Removing that session's container does
not remove your host files or named state volume. Do not remove `magent-state`
or use `docker compose down -v` unless you intend to discard saved logins/settings.
For an update, exit the old session, run `docker compose pull`, then start again.
Existing launcher users can keep their saved `magent-state` volume with Compose.
Docker may report that the volume was created outside Compose; it is reused.

### Linux Compose identity and AppArmor

Use your normal user's UID/GID to preserve ownership of host files. In the same
terminal, before the Compose commands:

```sh
export MAGENT_UID="$(id -u)" MAGENT_GID="$(id -g)"
```

The Linux website download selects `apparmor=magent-container-v1`. Load the named
profile using [Linux AppArmor setup](#linux-apparmor-setup). If Docker reports no
AppArmor support, remove only that AppArmor line from the downloaded configuration;
keep the seccomp and no-new-privileges settings. Windows/macOS downloads do not
select the Linux host profile. macOS Docker Desktop testing remains pending.

## Existing launcher users (optional)

The existing scripts remain compatible. They are no longer the recommended
website onboarding route. The instructions below are for users who already have
the launcher package or need the Linux host profile installer.

### Get the launchers

**Start Multiharness with the launcher in your terminal.** Docker Desktop's
generic **Run** dialog does not supply the required project mount, persistent
state, interactive terminal and sandbox configuration. The preview is a terminal
application; it has no browser dashboard or exposed web port.

1. Install and start Docker Desktop or Docker Engine.
2. Download the [v0.1.0-alpha.3 launcher ZIP](https://github.com/ErOr-0/Multiharness-Core/releases/download/v0.1.0-alpha.3/magent_docker_0.1.0-alpha.3.zip)
   and extract it outside your target project.
3. Open PowerShell on Windows, or Terminal on macOS/Linux, **inside the extracted
   folder containing `scripts`, `docker` and `docs`**. Keep those folders together.
4. Run first-time setup below with your workspace folder's full path. Finish the
   sign-in prompts, then run the separate launch command.

The launcher downloads `er0r2/multiharness-core:preview` automatically if the
image is missing. If you already pulled the image, use the same launcher steps.
The launcher supplies the project's bind mount, persistent volume and the scoped
Codex sandbox profile. Inspect [the profile notice](../docker/NOTICE.md) before
running it. Do not replace it with privileged mode or disable the agent sandbox.

## First run

From Windows PowerShell in the extracted launcher folder, replace the example
path with your workspace folder's full path. First-time setup:

```powershell
.\scripts\magent-docker.ps1 -Project 'D:\Projects\My App' -Command setup
```

After setup finishes, start Multiharness in that same terminal:

```powershell
.\scripts\magent-docker.ps1 -Project 'D:\Projects\My App'
```

On Linux with AppArmor (including Ubuntu 24.04), first complete the
[one-time host profile setup](#linux-apparmor-setup). macOS Docker Desktop does
not require that step.

On macOS/Linux, from the extracted launcher folder, run first-time setup:

```sh
sh ./scripts/magent-docker.sh --project '/path/to/My App' setup
```

After setup finishes, start Multiharness:

```sh
sh ./scripts/magent-docker.sh --project '/path/to/My App'
```

The interactive prompt appears in your terminal. Reuse the launch command for
later sessions; you do not need to repeat setup or sign-in while saved state is
available. Keep Docker running while using Multiharness.

The current directory is the default project when you omit `-Project` or
`--project`. Choose one project, a subfolder, or a parent folder containing
multiple projects and Git repositories. Git is optional; no `git init` is needed.
The launcher mounts the entire selected folder read/write at `/workspace`; edits
appear immediately on your computer. Paths in change reports include each project
folder. Existing uncommitted Git files remain protected, and the normal
review/repair loop still applies. See [workspace behavior](workspaces.md).

For example, `-Project 'D:\Projects\My Suite'` can share `api`, `web` and `tools`
together, even if each has its own Git repository or `tools` has none. Select the
folder containing the projects you want agents to access. Configure checks from
that folder, such as `npm --prefix web test` and `go -C api test ./...`.

If you have an older preview image, update it once before using folder support:

```text
docker pull er0r2/multiharness-core:preview
```

Saved provider logins can be reused. Linux AppArmor hosts need the updated
launchers and profile described below.

Docker Desktop may present host files as owned by a different Linux UID. The
container uses a temporary Git system-config overlay to trust `/workspace` and
repositories discovered beneath it. Your host Git configuration and persistent
global Git settings are unchanged; repositories outside the mount are not trusted.

`setup` checks the mount and actual Codex sandbox, then offers provider sign-in.
It does not call a model. Once back in `magent`, use `/config` to choose models
your accounts can access, then `/save`. Set project validation commands explicitly;
no checks run by default. A successful doctor check is not proof of tests or
provider entitlement.

OpenCode is optional. In `/config`, choose Codex for the planner and implementer,
then select their models independently (for example `gpt-6-astra` and
`gpt-5.6-luna`). The Codex reviewer also uses that saved Codex login. See
[Codex-only role configuration](https://github.com/ErOr-0/Multiharness-Core/blob/main/docs/cli.md#codex-implementation-without-opencode).
Decline the optional OpenCode setup prompt when you are not using it.

## Alternative: extract the launchers from the image

The image includes the same launcher package. To obtain it without a ZIP,
run these commands from a tools folder outside your target project:

```text
docker pull er0r2/multiharness-core:preview
docker create --name magent-launcher-download er0r2/multiharness-core:preview help
docker cp magent-launcher-download:/opt/magent/launcher ./magent-docker
docker rm magent-launcher-download
cd magent-docker
```

Choose another temporary container name if that name is already in use. After
`cd magent-docker`, follow the same first-run commands above. Source checkouts can
also use their existing `scripts/` and `docker/` folders.

## Linux AppArmor setup

Docker's default AppArmor profile denies the mount operations Codex needs to
build its inner sandbox. This can produce `bwrap: Failed to make / slave:
Permission denied` even with the supplied seccomp profile.

On a local Linux Docker host with AppArmor 4 or newer, obtain the current launcher
package using [image extraction](#alternative-extract-the-launchers-from-the-image).
The older v0.1.0-alpha.3 ZIP does not contain the AppArmor profile or installer.
From the updated package, review `docker/apparmor.profile`, then run once:

```sh
sudo sh ./scripts/magent-apparmor.sh
sh ./scripts/magent-docker.sh --project '/path/to/My App' doctor
```

The explicit host setup loads `magent-container-v1` and saves it in
`/etc/apparmor.d/magent-container-v1` for reboot. It does not change Docker's
default profile, other projects, host sysctls or AppArmor's enabled state. The
container still runs as your non-root user with all capabilities dropped and
`no-new-privileges`. Read-only planning/review remain enforced by Codex. See
[the profile notice](../docker/NOTICE.md) for the permissions and provenance.

The launcher detects AppArmor through Docker and selects that named profile.
If Docker reports that the profile is missing, perform the host setup above.
An older parser may reject `userns`; use a supported AppArmor 4 host rather than
removing the rule or disabling confinement. Windows/macOS Docker Desktop hosts
without AppArmor skip this profile automatically. No administrator command runs
during ordinary startup.

## Accounts and persistent settings

Each non-root user gets a private home under `/state/UID` in the named volume
`magent-state`. Credentials, provider sessions, caches and `/save` settings survive
container removal and image updates. Linux uses your host UID/GID so generated
files retain your ownership. Docker Desktop uses container UID 1000. Changing the
UID or volume selects separate state and may require signing in again.

The image does not contain anyone's credentials and does not import your host's
home folder, keychain, environment variables or provider settings.

- **Codex:** `login codex` runs device-code authentication. Open the printed link
  in your normal browser and sign into the same eligible ChatGPT account. Enable
  device login in account/workspace settings if required. File-based credentials
  are saved privately in the volume. API-key authentication is separately billed;
  it does not use included ChatGPT subscription access.
- **OpenCode:** `login opencode` runs the provider's own login menu. Available
  subscription/API-key methods depend on the selected provider. Use
  `opencode auth list` to inspect configured accounts. Some browser callback
  methods need provider-specific container networking; they are not universally
  verified by this package.

You can invoke provider commands through the same launcher. For example:

```powershell
.\scripts\magent-docker.ps1 -Project 'D:\Projects\My App' -Command @('login', 'codex')
.\scripts\magent-docker.ps1 -Project 'D:\Projects\My App' -Command @('opencode', 'auth', 'list')
```

```sh
sh ./scripts/magent-docker.sh --project '/path/to/My App' login codex
sh ./scripts/magent-docker.sh --project '/path/to/My App' opencode auth list
```

If device login is unavailable, the official Codex guide describes copying a
file-based auth cache into a trusted container. OS-keychain credentials cannot
be copied that way. Do not put auth files in the project, Dockerfile, image or
Git; use a private state volume and allow token refresh. This preview does not
automatically import or overwrite host authentication.

References: [Codex authentication](https://learn.chatgpt.com/docs/auth),
[OpenCode CLI](https://opencode.ai/docs/cli/#auth).

## Project tools

The image bundles Git, Codex 0.153.0, OpenCode 1.18.23, Node 24/npm, Go 1.26.6,
Python 3, Make, GCC/G++, curl, SSH client and ripgrep. Docker base images use
multi-platform digest pins. Agent packages have exact version pins; Debian
packages and npm transitive resolution are not a fully reproducible lockfile.

Installed host tools are separate. For example, .NET installed on Windows will
not make `dotnet test` available in this Linux container. Check explicitly:

```sh
sh ./scripts/magent-docker.sh --project '/path/to/My App' doctor dotnet
```

For other toolchains, build a derived image with the required Linux SDK and
select it with `--image my-magent:dev` or PowerShell `-Image my-magent:dev`.
Restore `USER 1000:1000` after any root-only image build steps. Package managers
must install Linux dependencies; host `node_modules`, virtual environments and
compiled native binaries may be incompatible. Windows-only build targets still
need a Windows build environment.

`shell` opens Bash with the same project, state and tools for troubleshooting.
For scripted tasks, use `--no-tty -- --task '...'` on macOS/Linux, or PowerShell
`-NoTty -Command @('--task', '...')`. The existing JSON result and exit codes are
preserved. Saved interactive settings apply to interactive launches; scripted
runs should select configuration explicitly with the normal CLI flags.

## Boundaries and troubleshooting

- The launchers use a local Docker engine. A remote Docker context cannot mount
  a folder from your laptop automatically.
- Only the selected project is shared. Sibling repositories, host services,
  SSH agents and other files require deliberate additional configuration.
- Linked worktrees with Git metadata outside the project mount need that metadata
  accessible at compatible paths. A standalone clone is the simplest setup.
- Docker Desktop file sharing must allow the selected directory. Case sensitivity,
  symlinks, executable bits and file-lock behavior depend on the host filesystem.
  The launchers reject commas and quotes in project paths; spaces are supported.
- If Codex cannot create its sandbox, run `doctor` using the supplied launcher.
  Additional host restrictions may still block namespaces. The package does not
  automatically modify host policies or fall back to unrestricted agent access.
- Private state persists; deleting `magent-state` deletes its saved logins and
  settings. Image removal alone does not delete the volume. Back up state privately.
- No automatic rollback or durable workflow resumption is added. Cancellation
  leaves project changes for inspection, as in the native CLI.

## Build, test and publish

Windows Docker Desktop amd64 container checks pass. GitHub Ubuntu 24.04 native
amd64 and arm64 runners pass actual AppArmor/Codex sandbox checks, mounted folder
and nested Git operations, persistent state, and the full offline Go suite:
[verified Linux run](https://github.com/ErOr-0/Multiharness-Core/actions/runs/34196727275).
macOS Docker Desktop testing is deferred until a real Mac is available; native
macOS Go CI does not establish Docker Desktop compatibility. Keep live-provider
completion evidence separate from these offline checks.

Authenticated Docker verification on 2026-09-08 passed immediate approval and a
real reject/repair/approval cycle with Codex 0.153.0: `gpt-6-astra` planning,
`gpt-5.6-luna` implementation, and `gpt-5.6-sol` independent review, all with
`xhigh` reasoning. Both used disposable repositories, real Go checks and
independent change attribution while preserving existing notes. Codex timeout
and cancel-after-output checks also passed. OpenCode 1.18.23 lifecycle probes
passed separately; its full workflow could not complete with the rejected Zen
test credential. Other providers/models and macOS Docker remain unverified.

```sh
docker build -t multiharness-core:dev .
python3 scripts/test-docker.py multiharness-core:dev
docker run --rm --cap-drop ALL --security-opt no-new-privileges=true \
  --mount "type=bind,src=$PWD,dst=/source,readonly" \
  --entrypoint /bin/sh multiharness-core:dev /source/docker/check-source.sh
```

The smoke script uses a disposable Git project and private temporary volume. It
tests mounted edits, retained state, non-root execution, setup failures, missing
tools and actual Codex sandbox permissions without authenticating or calling a
model. The source check runs `make fmt`, full offline tests/race/fuzz/static checks
and workflow lint on a disposable source copy.

The **Docker image** workflow runs native amd64/arm64 container checks. To publish,
configure the `dockerhub` GitHub environment with `DOCKERHUB_USERNAME` and a scoped
`DOCKERHUB_TOKEN`, then dispatch the workflow from `main` with `publish=true` and
a unique `preview-*` tag. Publishing waits for both architecture checks; the
combined tag and `preview` alias are only created after both uploads succeed.
No `latest` or stable tag is published. Keep the existing authenticated and
cross-host release gates separate from offline packaging tests.
