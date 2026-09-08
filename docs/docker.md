# Run Multiharness in one Docker container

Image: `er0r2/multiharness:latest` (Linux amd64 and arm64).
Container: `multiharness`. Saved accounts and settings: volume `magent-state`.

Docker Desktop or a local Linux Docker Engine must be running. Windows uses Linux
containers. The application is a terminal interface; it does not expose a web port.

## One-time setup

Install and open Docker Desktop first. Windows uses Linux containers. On native
Linux, install Docker Engine and the Compose plugin; `docker info` must work as
your regular user.

1. Download the [setup ZIP](https://multiharness.mdfahimhossen.space/downloads/multiharness-docker.zip)
   and extract its contents to `~/multiharness` (macOS/Linux) or
   `C:\Users\YOUR_NAME\multiharness` (Windows). The `scripts` folder and
   `compose.yaml` should be directly inside that folder.
2. Run the setup command below. It asks for the host folder containing your
   projects and saves your answer automatically. It then downloads the image,
   creates one named container and opens the app.
3. Choose a project inside the shared folder. If no team settings exist, the app
   asks for your agents and models. Completed answers save automatically.
   Sign in with `/login codex` and, if selected, `/login opencode`.
   Then type a task, such as **Explain this project and how to run it.**

**macOS / Linux — Terminal:**

```sh
bash "$HOME/multiharness/scripts/setup.sh"
```

**Windows — PowerShell:**

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File "$env:USERPROFILE\multiharness\scripts\setup.ps1"
```

The Windows execution-policy option applies only to this setup process; it does
not change your computer's policy. The scripts are included in the download for
inspection. They install no host `magent` command and do not modify your PATH.

Setup uses the canonical Compose definition and stores the shared folder in
`.env` automatically. Existing settings are preserved. On native Linux it uses
your UID/GID and installs the scoped AppArmor profile when needed (with sudo).
If a container is already running, it tells you how to attach without interrupting
that session. Keep this setup folder for updates.

The first download may take a few minutes. Later starts restore your saved
project and team without repeating setup. `/cancel` discards an incomplete team
setup; it does not launch an agent or save partial team answers.

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

Accounts and setup commands run inside this container; they do not create helper
containers. Credentials remain under `/state/<uid>` and are not part of the image.

## Updates

Type `/quit`, then run the setup command again. It reuses your saved folder,
pulls the current image and recreates only the named container when needed.
The `magent-state` volume and your original files are retained.

To change projects **within** the shared host folder, use `/config` → **1**.
Docker records the shared host folder when creating the container. To share a
different host folder, quit, rename `.env` in the setup folder to `.env.backup`,
then run setup again. It asks for the new folder and reuses the same state volume.
If the saved in-container project is missing, the app asks you to select one.
Do not delete `magent-state` or use `down -v` to update.

For manual administration, the underlying commands remain standard Docker:

```sh
docker pull er0r2/multiharness
docker compose --env-file "$HOME/multiharness/.env" -f "$HOME/multiharness/compose.yaml" up --no-start
docker start -ai multiharness
```

Use PowerShell paths on Windows and the Linux override below on AppArmor hosts.

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
