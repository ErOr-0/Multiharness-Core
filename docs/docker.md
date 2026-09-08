# Run Multiharness in one Docker container

Image: `er0r2/multiharness:latest` (Linux amd64 and arm64).
Container: `multiharness`. Saved accounts and settings: volume `magent-state`.

Docker Desktop or a local Linux Docker Engine must be running. Windows uses Linux
containers. The application is a terminal interface; it does not expose a web port.

## One-time setup

Follow these four steps on your own computer. On Windows, use **PowerShell**;
on macOS or Linux, use **Terminal**. You can run the commands from any folder.
On Linux, make sure both Docker Engine and the Compose plugin are installed and
`docker info` works as your regular user.

### 1. Download and extract

Download the [configuration ZIP](https://multiharness.mdfahimhossen.space/downloads/multiharness-docker.zip)
and extract its contents into:

- Windows: `C:\Users\YOUR_NAME\multiharness` (replace `YOUR_NAME`).
- macOS or Linux: `~/multiharness` (`~` means your home folder).

Check that `compose.yaml` is directly inside this folder, not inside an extra
nested folder. Keep this configuration folder for future updates.

### 2. Choose the folder to share

Create and open `.env` using the command for your operating system. These
commands preserve an existing `.env` file when you already have one.

**Windows — PowerShell:**

```powershell
if (!(Test-Path "$env:USERPROFILE\multiharness\.env")) { Copy-Item "$env:USERPROFILE\multiharness\.env.example" "$env:USERPROFILE\multiharness\.env" }
notepad "$env:USERPROFILE\multiharness\.env"
```

**macOS — Terminal:**

```sh
test -f "$HOME/multiharness/.env" || cp "$HOME/multiharness/.env.example" "$HOME/multiharness/.env"
open -e "$HOME/multiharness/.env"
```

**Linux — Terminal:**

```sh
test -f "$HOME/multiharness/.env" || cp "$HOME/multiharness/.env.example" "$HOME/multiharness/.env"
nano "$HOME/multiharness/.env"
```

In the editor, replace `MULTIHARNESS_WORKSPACE` with the full path to an **existing
project folder**. For example, on Windows:

```dotenv
MULTIHARNESS_WORKSPACE='D:/Projects'
```

On macOS, a path might be `'/Users/YOUR_NAME/Projects'`; on Linux,
`'/home/YOUR_NAME/Projects'`. Use your actual path, keep the single quotes, and
save as `.env`, not `.env.txt`. Windows paths can use forward slashes as shown.
A parent folder can contain several projects. Agents edit the original files in
this shared folder.

Docker Desktop users leave the UID/GID values at `1000`. Native Linux users set
`MULTIHARNESS_UID` and `MULTIHARNESS_GID` to the numbers printed by `id -u` and
`id -g`. In nano, save with Ctrl+O, Enter, then exit with Ctrl+X.

### 3. Create and start your container

Copy and run each command in order. The first download may take a few minutes.

**macOS / Linux without AppArmor:**

```sh
docker pull er0r2/multiharness
docker compose --env-file "$HOME/multiharness/.env" -f "$HOME/multiharness/compose.yaml" up --no-start
docker start -ai multiharness
```

**Windows — PowerShell:**

```powershell
docker pull er0r2/multiharness
docker compose --env-file "$env:USERPROFILE\multiharness\.env" -f "$env:USERPROFILE\multiharness\compose.yaml" up --no-start
docker start -ai multiharness
```

**Linux with AppArmor:** check the Security Options section of `docker info`.
If it lists `apparmor` (common on Ubuntu), use this sequence instead:

```sh
sudo sh "$HOME/multiharness/scripts/magent-apparmor.sh"
docker pull er0r2/multiharness
docker compose --env-file "$HOME/multiharness/.env" -f "$HOME/multiharness/compose.yaml" -f "$HOME/multiharness/docker/compose.linux.yaml" up --no-start
docker start -ai multiharness
```

You should see the Multiharness welcome screen and a folder picker. You now have
one container named `multiharness`. Pull downloads the image; Compose records
your shared folder and persistent settings; start opens the application.

### 4. Sign in and send your first task

Press Enter to use the shared folder, or select a project inside it. At the
application's prompt, enter these commands **one at a time**:

1. `/login codex` — follow the sign-in instructions shown.
2. `/config` — choose agents and models. If you choose OpenCode, also run
   `/login opencode` and follow its sign-in instructions.
3. `/save` — remember your team.
4. Type a task, for example: **Explain this project and how to run it.**

Your selected project, logins and settings are remembered. When finished, type
`/quit`. Next time you only need the everyday start command below.

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
