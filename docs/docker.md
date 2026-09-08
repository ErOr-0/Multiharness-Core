# Run Multiharness with Docker

The preview image is `er0r2/multiharness-core:preview`, with Linux amd64 and
arm64 variants. Docker selects the architecture. This runs locally with your
own provider accounts; Docker Hub distributes the software only.

Install and start Docker Desktop on Windows/macOS, or Docker Engine on Linux.
Windows uses Linux containers; Docker Desktop commonly still uses WSL 2 behind
the scenes. You can run the launcher from PowerShell without installing Git,
Codex, OpenCode, Go, or Node on Windows. Native Windows `magent.exe` execution
remains unsupported.

## Get the launchers

Use the `magent_docker_VERSION.zip` from GitHub Releases when available, or use
`scripts/` and `docker/` from this source repository. Keep their relative paths.
The launcher supplies the project's bind mount, persistent volume and the scoped
Codex sandbox profile. Inspect [the profile notice](../docker/NOTICE.md) before
running it. Do not replace it with privileged mode or disable the agent sandbox.

The image also includes the launcher package. Extract it without starting a task:

```text
docker pull er0r2/multiharness-core:preview
docker create --name magent-launcher-download er0r2/multiharness-core:preview help
docker cp magent-launcher-download:/opt/magent/launcher ./magent-docker
docker rm magent-launcher-download
```

Choose another temporary container name if that name is already in use.

## First run

From Windows PowerShell, point the launcher at your Git repository:

```powershell
.\magent-docker\scripts\magent-docker.ps1 -Project 'D:\Projects\My App' -Command setup
.\magent-docker\scripts\magent-docker.ps1 -Project 'D:\Projects\My App'
```

On macOS/Linux:

```sh
sh ./magent-docker/scripts/magent-docker.sh --project '/path/to/My App' setup
sh ./magent-docker/scripts/magent-docker.sh --project '/path/to/My App'
```

The current directory is the default project when you omit `-Project` or
`--project`. The folder must be a Git repository root. The launcher mounts it
read/write at `/workspace`; edits appear immediately on your computer. Existing
dirty-file protection and the normal review/repair loop still apply.

`setup` checks the mount and actual Codex sandbox, then offers provider sign-in.
It does not call a model. Once back in `magent`, use `/config` to choose models
your accounts can access, then `/save`. Set project validation commands explicitly;
no checks run by default. A successful doctor check is not proof of tests or
provider entitlement.

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
.\magent-docker\scripts\magent-docker.ps1 -Command @('login', 'codex')
.\magent-docker\scripts\magent-docker.ps1 -Command @('opencode', 'auth', 'list')
```

```sh
sh ./magent-docker/scripts/magent-docker.sh login codex
sh ./magent-docker/scripts/magent-docker.sh opencode auth list
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
sh ./magent-docker/scripts/magent-docker.sh doctor dotnet
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

Local verification on Windows Docker Desktop covers Linux amd64, including real
Codex sandbox commands and the full offline Go suite. The ARM64 image was built
and its tools started under CPU emulation; namespace sandboxing requires native
ARM hardware and is not verified by that emulation. Native macOS/Linux desktop
mount behavior, authenticated provider login/completion and remote CI remain
release gates. This image is a preview, not a claim that those gates passed.

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
