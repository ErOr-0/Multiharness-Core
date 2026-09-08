# Run Multiharness in one Docker container

Image: `er0r2/multiharness:latest` (Linux amd64 and arm64).
Container: `multiharness`. Saved accounts and settings: volume `magent-state`.

Docker Desktop or a local Linux Docker Engine must be running. Windows uses Linux
containers. The application is a terminal interface; it does not expose a web port.

## One-time setup

1. Download the [configuration bundle](https://multiharness.mdfahimhossen.space/downloads/multiharness-docker.zip)
   and extract it to a permanent folder, such as `~/multiharness` or
   `C:\Users\yourname\multiharness`. No host executable is installed.
2. Copy `.env.example` to `.env` beside `compose.yaml`. Set
   `MULTIHARNESS_WORKSPACE` to an existing absolute host folder. For example,
   `'/Users/yourname/Projects'` or `'D:/Projects'`. Spaces and `$` work inside the
   single quotes. Select only the folder you want to share; original files there
   are edited directly. A parent folder can contain multiple projects.
3. On native Linux, set `MULTIHARNESS_UID` and `MULTIHARNESS_GID` to the output of
   `id -u` and `id -g`. For AppArmor hosts, follow the Linux section below.
4. Pull and create the named container. Replace the absolute example paths below.
   You can run these commands from any current directory.

```sh
docker pull er0r2/multiharness
docker compose --env-file "$HOME/multiharness/.env" -f "$HOME/multiharness/compose.yaml" up --no-start
docker start -ai multiharness
```

Windows PowerShell creation command:

```powershell
docker compose --env-file "$env:USERPROFILE\multiharness\.env" -f "$env:USERPROFILE\multiharness\compose.yaml" up --no-start
```

Pull only downloads the image. Creating the container records the host mount,
state volume, terminal and sandbox settings once. Keep the configuration folder
for updates; everyday starts do not depend on being in that folder.

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
- `/config` chooses your team; `/save` remembers the team settings.
- Type a task to begin. No validation checks run unless you configure them.
- `/quit` stops the application, retaining the container, files and settings.

If already running, reconnect with `docker attach multiharness`. Use one attached
interactive client at a time. Docker's Ctrl+P, Ctrl+Q detach keys leave it running;
Ctrl+C cancels active work and exits. Docker Desktop can start/stop the same named
container. To interact, attach from a terminal; no browser window is opened.

Accounts and setup commands run inside this container; they do not create helper
containers. Credentials remain under `/state/<uid>` and are not part of the image.

## Updates

Exit the application first. Pull the new image, then let the saved Compose
definition replace the stopped container. Reuse its bind mount and state volume:

```sh
docker pull er0r2/multiharness
docker compose --env-file "$HOME/multiharness/.env" -f "$HOME/multiharness/compose.yaml" up --no-start
docker start -ai multiharness
```

On Windows, use the same PowerShell absolute paths as in setup. The container ID
changes when applying a new image; the name and volume stay the same. Pulling an
image or simply restarting an old container does not update that container.
Do not use `down -v` or delete `magent-state` to update.

To share a different host folder, edit `.env` and repeat the creation command while
stopped. Projects inside the existing shared tree only require `/workspace`.

## Linux AppArmor setup

The scoped policy files are required by the current Codex sandbox. They are host
configuration, not additional containers. On native Linux with AppArmor, load the
included profile once using its absolute path:

```sh
sudo sh "$HOME/multiharness/scripts/magent-apparmor.sh"
```

Then include the small Linux policy override every time you create/update:

```sh
docker compose --env-file "$HOME/multiharness/.env" -f "$HOME/multiharness/compose.yaml" -f "$HOME/multiharness/docker/compose.linux.yaml" up --no-start
```

Everyday start remains `docker start -ai multiharness`. Docker Desktop users do
not need the native-Linux override. The profile does not change the global Docker
policy, host sysctls or other containers. See `docker/NOTICE.md` for provenance.

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

`make package-docker` builds the configuration ZIP using the maintained files.
`python3 scripts/test-docker.py IMAGE` exercises the actual image and reusable
container lifecycle in isolated temporary resources. The Docker workflow checks
native architectures before publication; `make check` covers the Go workflow.
