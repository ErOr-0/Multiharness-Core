# Docker standardization plan

Status: approved and implemented in source, 2026-09-08. Local offline Go,
Docker Desktop ARM64 lifecycle/sandbox and website checks passed. Publication,
native Linux CI and deployment evidence are recorded in AGENTS.md when complete.

## Intended experience

One published image, `er0r2/multiharness`, and one reusable container named
`multiharness`. Installation happens once; everyday use does not depend on the
current directory, an extracted archive, a repository checkout or a host `magent`
executable. Keep the existing terminal application and its workflow service.

`docker pull er0r2/multiharness` downloads `:latest`. Docker pull cannot execute
installation code or create/start containers. Initial creation must also declare
the shared host folder, persistent state and required sandbox policy. Make this
constraint explicit instead of claiming that pull installs a host command.

After initial creation, the everyday command from any directory is:

```sh
docker start -ai multiharness
```

`/quit` stops the application and leaves the same container available for the
next start. It does not delete the container, state or project files. If already
running, attach to it with `docker attach multiharness`; do not create another.
Docker Desktop can manage the same named container. This remains a terminal app,
not a web service; starting a container does not open a browser interface.

## 1. Establish one lifecycle

- Introduce a canonical, ordinary YAML `compose.yaml` with one service and
  `container_name: multiharness`, `stdin_open: true`, `tty: true` and `init: true`.
- Keep the application as the main process. No supervisor, background sleep,
  helper container, Docker socket, web server or orchestration framework.
- Set the image to `er0r2/multiharness:latest`; publish immutable version tags too.
  `latest` denotes the current supported download, not completion of existing
  authenticated release gates.
- Keep one host bind at `/workspace` and reuse `magent-state` at `/state` so
  existing credentials/settings survive. These are storage mounts, not containers.
- Use an absolute host folder in the one-time configuration. A parent folder can
  contain several projects; `/workspace` selects a project within that mount.
- Use `docker compose -f /absolute/path/compose.yaml up --no-start` for initial
  creation, followed by `docker start -ai multiharness`. The downloaded bundle
  contains the canonical Compose file and required policy, not executable launchers.
  Document corresponding Windows absolute paths.
- Avoid `docker run --rm` and `docker compose run --rm` in normal user instructions.
  A stopped named container is reusable; a name collision should explain how to
  inspect/reuse it, not choose a random new name.
- Preserve platform UID/GID behavior. Changing to a different host mount requires
  recreating this one container; ordinary project selection under its mount does not.

## 2. Bring onboarding into that container

- Keep team configuration, account setup and workspace selection in the container's
  terminal flow. Consolidate the existing entrypoint setup/login commands and the
  interactive application instead of maintaining a second host configuration UI.
- Reuse current provider CLI login implementations without exposing credentials.
  Setup and login must run as child processes in this same container.
- Persist and validate the last selected workspace within the mounted root, so
  repeat starts do not require selecting the same folder again. Clear provider
  session IDs appropriately when switching projects.
- Replace `MAGENT_HOST_LAUNCHER` branches and host-launcher help with one startup
  behavior. Keep the mount-boundary checks and normal typed `Service.Run` entry.
- Retain unlimited workspace size/count defaults and existing user settings.
  Existing positive inspection caps can be cleared explicitly; migrate this user's
  already-cleared configuration without restoring old caps.

## 3. Remove duplicate delivery paths

Delete only after references and their replacement behavior are covered:

| Remove or simplify | Replacement |
| --- | --- |
| `cmd/magent-launcher/`, `internal/launcher/` | Standard Docker lifecycle; onboarding inside the container |
| `scripts/magent-docker.sh`, `scripts/magent-docker.ps1` | One Compose definition and standard Docker commands |
| `scripts/build-host-launchers.py` | Docker image plus a small configuration release bundle |
| `scripts/generate-compose.py` | One maintained Compose source, copied unchanged into release assets |
| `docker/assets.go` | No host executable embedding a duplicate policy |
| `web/src/docker-setup.js` | Static configuration-bundle download; remove handwritten ZIP generation |
| Generated `web/public/compose.yaml` and policy copies | Release bundle assembled from the canonical files |
| Host-launcher download buttons and obsolete static ZIPs | One Docker onboarding path |
| `docs/host-launcher.md` and overlapping launcher sections | One `docs/docker.md` guide plus a short migration note |
| Host/native competing `magent` release archives in `.goreleaser.yml` | Docker-first publication; retain development builds separately |
| Launcher-specific tests and assertions | Container lifecycle, onboarding and persistence tests |

Keep `cmd/multiharness`, the workflow core, provider adapters and source development
commands. Do not remove files merely because their names contain Docker. Review
Makefile installation guidance so it cannot silently put an incompatible native
`magent` command back on a Docker user's PATH.

## 4. Keep only required platform configuration

Retain `Dockerfile`, `.dockerignore`, the focused entrypoint, one Compose source,
container tests and one Docker publication workflow. Remove launcher copies from
the runtime image once their entrypoints and downloads are gone.

Retain `docker/seccomp.json`, its license/notice and the Linux AppArmor profile
and installation helper while needed for the supported Codex sandbox. Docker
applies those policies when creating a container; putting a profile inside the
image does not activate it. Their removal requires successful tests with Docker's
default policies on supported native hosts. Do not replace them with privileged
mode, unconfined policies or disabling the agent sandbox merely to shorten setup.

Review `.goreleaser.yml` and `.github/workflows/release.yml`: remove launcher/native
packaging jobs; retain only nonduplicated release duties. Assemble the configuration
bundle using a standard archive tool, not custom ZIP serialization.

## 5. Standardize publishing and migration

- Publish checked linux/amd64 and linux/arm64 images to `er0r2/multiharness` with
  immutable version tags plus `latest`. Verify the registry index after push.
- Build releases from a recorded source commit using the canonical Dockerfile.
  Stop using ad hoc local binary layers as the routine publication path.
- Update CI filters so runtime Go changes trigger container checks, not only
  Docker/script changes. Fix the build-context allowlist to include all files
  actually copied by the Dockerfile, including AppArmor assets.
- Update README, Docker Hub description, website and release guide together.
  The old repository can retain a documented compatibility tag during migration;
  it must not remain a second onboarding choice.
- Stop an existing user's session gracefully. Inspect specifically identified
  Magent containers before replacing them; do not prune unrelated containers,
  images or volumes. Reuse `magent-state` and the original host bind.
- Handle obsolete host binaries with a migration notice or backed-up removal
  within the user's explicitly selected installation. Do not delete unrelated
  executables or all downloaded archives.
- Updates pull the new image, stop/recreate the one named container using the
  saved Compose definition, then start it again. Pulling or restarting an existing
  container alone does not change its image. Reuse volumes and bind mounts.

## 6. Completion evidence

- Fresh setup produces exactly one application container named `multiharness`.
- Repeated starts from unrelated working directories keep the same container ID;
  setup, login, configuration and tasks create no additional application containers.
- `/quit`, restart and Docker Desktop restart preserve the host files, credentials,
  team settings and selected in-mount workspace. Detached/running-session behavior
  is documented and tested without allowing accidental duplicate task submission.
- An image update replaces only that container and retains the same state volume.
- Plain folders, large workspaces and multiple repositories retain complete evidence.
  Planner/reviewer restrictions and repair/cancellation behavior still pass.
- Verify real PTY startup and repeated attachment, plus required sandbox checks on
  native amd64 and arm64 hosts. Emulated sandbox failures are not native verification.
- Run `make fmt`, applicable offline Go/integration/race/static checks, container
  lifecycle tests, workflow lint and website content/build checks after the cleanup.
  No authenticated model calls are required for this delivery change.
- Inspect the repository for old launch commands, image names, download links and
  deleted-file references. Publish only the resulting checked build.

## Implementation order

1. Implement and verify named-container lifecycle and in-container onboarding.
2. Remove redundant launchers/generators and update the website/docs/release path.
3. Verify migration and supported host behavior, then publish the new canonical image.

Keep existing user source changes throughout. Update the implementation checklist
with actual evidence at each phase gate; this plan does not mark Phase 9 complete.

References:

- Docker pull downloads images: https://docs.docker.com/reference/cli/docker/image/pull/
- Restart and attach to an existing container: https://docs.docker.com/reference/cli/docker/container/start/
- Compose creation and update behavior: https://docs.docker.com/reference/cli/docker/compose/up/
- Host folder mounting: https://docs.docker.com/engine/storage/bind-mounts/
- Host-applied seccomp configuration: https://docs.docker.com/engine/security/seccomp/
