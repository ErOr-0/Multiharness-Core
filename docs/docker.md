# Run Multiharness in one Docker container

Image: `er0r2/multiharness:latest` (Linux amd64 and arm64).
Container: `multiharness`. Saved accounts and settings: volume `magent-state`.

Docker Desktop or a local Linux Docker Engine must be running. Windows uses Linux
containers. The application is a terminal interface; it does not expose a web port.

## First run — Docker commands, no setup script

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
   and, if selected, `/login opencode`, then type your task.

Docker must receive the host folder at creation time. `/config` can then change
projects within that shared folder; it cannot attach another host folder to an
already-created container. `/cancel` discards incomplete team configuration.

## Everyday use — from any directory

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

## Updates

Type `/quit`. Run `docker pull er0r2/multiharness`, then repeat your first launch
command with the **same host folder** and Linux policy choice. Compose replaces
only the named container when needed and retains the `magent-state` volume.
Existing app settings and provider logins load from that volume. Use the same UID
on Linux. Never delete `magent-state` or use `down -v` to update.

Use `/config` → **1** to switch projects inside the shared folder. To share a
new host folder, quit and repeat the launch command with its absolute path. This
recreates the same named service with the new mount and existing state. If the
saved project is unavailable, the app asks you to choose one again.

## Linux AppArmor setup

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
filters as a workaround. See `docker/NOTICE.md` for provenance.

Include the Linux override in the first launch command and future updates:

```sh
MULTIHARNESS_WORKSPACE='/path/to/Projects' MULTIHARNESS_UID="$(id -u)" MULTIHARNESS_GID="$(id -g)" docker compose -f 'https://github.com/ErOr-0/Multiharness-Core.git#main' -f 'https://github.com/ErOr-0/Multiharness-Core.git#main:docker/compose.linux.yaml' create &&
docker start -ai multiharness
```

The website includes this override when the AppArmor option is selected.
Everyday start remains `docker start -ai multiharness`.

## Migration from older versions

Older builds used host `magent` launchers and random disposable containers.
Exit those sessions, then use the setup above. Keep `magent-state` and use the same
UID to retain login and team settings. Back up any old host `magent` executable
before removing it from PATH; it is no longer part of the supported Docker flow.
The previous `er0r2/multiharness-core` repository is historical, not the canonical
image. Do not mix its launcher/configuration with this installation.

A pre-existing unrelated container named `multiharness` is a name collision:
inspect it before making changes. Never prune unrelated containers or volumes.

## Troubleshooting and boundaries

- `docker ps -a --filter name=multiharness` shows the saved container.
- While it is running, `docker exec -it multiharness magent-container doctor`
  checks bundled tools and the real sandbox without model calls.
- Invalid mount paths fail rather than silently creating a new host folder.
- Docker can browse only the shared tree. It cannot select arbitrary host folders
  from inside the container, and a remote Docker engine cannot mount client files.
- Workspace sizes/file counts are unlimited by default. Snapshot memory use grows
  with the selected folder. For a slow Docker bind mount, use
  `/set git-timeout 5m` and `/save` to give inspection more time.
- The image includes Codex 0.153.0, OpenCode 1.18.23, Go 1.26.6, Node 24,
  Python, Git, Make and C/C++ tools. Project-specific SDKs may still be needed.
- Planning/review remain read-only and implementation uses the selected provider's
  configured permissions. Dirty Git files are protected. Ignored artifacts and
  effects outside the inspected tree are outside change attribution.
- No Docker socket, privileged mode, extra service or automatic host installation
  is required. Do not disable the agent sandbox to work around a failed check.
- The release is still a preview while the existing live release gates remain
  open. Emulation is not native sandbox verification.

## Maintainer checks

`make package-docker` builds an optional offline configuration ZIP with no setup
scripts. It is not required by the normal Docker-native installation.
`python3 scripts/test-docker.py IMAGE` exercises the actual image and reusable
container lifecycle in isolated temporary resources. The Docker workflow checks
native architectures before publication; `make check` covers the Go workflow.

If a shared parent folder contains protected child directories (for example a
local database data directory), startup skips children the container user cannot
read or traverse when preparing Git access. It does not change their permissions.
Choose an accessible project folder for tasks; a task that actually requires a
protected path can still report a permission error.
