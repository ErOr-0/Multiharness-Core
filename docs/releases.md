# Downloads and releases

For the bundled Docker preview, download a **magent-host** ZIP from
[the website](https://multiharness.mdfahimhossen.space/#start) and follow
[the host launcher guide](host-launcher.md). It includes a native launcher for
Windows, macOS or Linux. Use `magent --config` to choose Folder, Models or
Accounts; the launcher attaches your original folder to Docker automatically.
Future tagged releases also package these ZIPs. Older releases may only contain
the advanced `magent_docker_VERSION.zip` PowerShell/Compose starter.

## Install the standalone workflow binary (without Docker)

Open [GitHub Releases](https://github.com/ErOr-0/Multiharness-Core/releases), select
a published release and download an archive plus `checksums.txt`. Draft releases
are visible to maintainers and are not public downloads. If no release is listed,
use the source installation instructions in the [README](../README.md).

| Your system | Archive suffix |
| --- | --- |
| macOS, Apple Silicon | `_darwin_arm64.tar.gz` |
| macOS, Intel | `_darwin_amd64.tar.gz` |
| Linux, Intel/AMD 64-bit | `_linux_amd64.tar.gz` |
| Linux, ARM64 | `_linux_arm64.tar.gz` |
| Windows with WSL | Choose the Linux archive matching the architecture inside WSL |

`darwin` is Go's name for macOS. Use `uname -m` in your terminal to check the
architecture: `x86_64` corresponds to `amd64`; `aarch64` or `arm64` corresponds
to `arm64`. The standalone workflow binary requires Linux or macOS; the separate
Docker host launcher supports Windows too. WSL users running the standalone
binary must install the agent tools inside Linux.

Verify the archive's SHA-256 hash against its line in `checksums.txt`:

```sh
# Replace ARCHIVE.tar.gz with the downloaded filename.
# macOS:
shasum -a 256 ARCHIVE.tar.gz
# Linux / WSL:
sha256sum ARCHIVE.tar.gz
```

Extract into a new directory, then install the binary in your personal PATH:

```sh
mkdir magent-download
tar -xzf ARCHIVE.tar.gz -C magent-download
mkdir -p "$HOME/.local/bin"
install -m 755 magent-download/magent "$HOME/.local/bin/magent"
export PATH="$HOME/.local/bin:$PATH"
magent --version
```

Add the PATH line to your shell profile for future sessions. On macOS, these
archives are not Apple Developer ID signed or notarized. If macOS blocks the
downloaded program, review its origin and use the normal Privacy & Security
approval flow; see [Apple's instructions](https://support.apple.com/en-us/102445).

Authenticated Codex/OpenCode CLIs remain separate prerequisites. Git is optional. A binary
download includes the orchestrator, documentation and an example configuration;
it does not include models, provider accounts or your project's build tools.
Go is needed only when building magent from source or running Go-based project checks.

Run the current `magent` build inside your project folder, use `/config` to choose your team, then
`/save` to remember it. See the [README](../README.md) for the normal task flow.

## Prepare a release as a maintainer

The repository uses [GoReleaser](https://goreleaser.com/customization/ci/actions/)
with `.goreleaser.yml`. GoReleaser v2.18.1 and the GitHub Actions are pinned in CI.
Normal branch/PR checks build snapshot archives without publishing. They verify
checksums and execute the host-compatible packaged binary's `--version` and
`--help` on Linux and macOS. Other architectures are cross-built; this is not a
claim of authenticated provider compatibility on every target.

The application remains pre-release while the documented Phase 9 gates are open.
Use a prerelease tag for an explicitly limited preview and describe unverified
behavior in its notes. Complete [release-readiness.md](release-readiness.md)
before claiming a stable release.

1. Review and land the intended changes through the normal PR process. The release
   includes committed files only; local uncommitted work is not included.
2. Select a semantic version. For example, `v0.1.0-alpha.1` identifies a preview;
   it is an example, not a tag created by this change. Check that it is unused.
3. From the intended clean release commit, create and push that tag:

   ```sh
   git tag -a v0.1.0-alpha.1 -m "magent v0.1.0-alpha.1"
   git push origin v0.1.0-alpha.1
   ```

4. The **Draft release** workflow calls the existing deterministic, security and
   packaging checks on that same tag commit. Only after every job succeeds does
   it build the release archives and upload them to a **draft GitHub Release**.
   It uses the repository's built-in `GITHUB_TOKEN` with `contents: write` only in
   the upload job; provider credentials are not needed or passed to CI.
5. Open the draft, inspect all four archives and `checksums.txt`, and replace the
   maintainer reminder with actual tested OS/architecture, CLI versions, models,
   permissions, outcomes and known limitations. Complete the applicable live
   compatibility checks; the workflow deliberately makes no model calls.
6. Publish the reviewed draft in GitHub. Only then can users download its assets
   from the release page. See [GitHub's release guide](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository).

The resulting names resemble:

```text
magent_0.1.0-alpha.1_darwin_amd64.tar.gz
magent_0.1.0-alpha.1_darwin_arm64.tar.gz
magent_0.1.0-alpha.1_linux_amd64.tar.gz
magent_0.1.0-alpha.1_linux_arm64.tar.gz
checksums.txt
```

Drafts remain unpublished even for stable-looking tags. A successful upload does
not complete the release-readiness checklist. Do not move an already published
tag to different code; fix forward with a new version.

## Validate packaging locally

Install the pinned GoReleaser version from its official distribution. With the
project's declared Go toolchain selected, run:

```sh
make fmt
make check
make security lint-workflows
goreleaser check
goreleaser release --snapshot
```

Snapshot output goes to `dist/release`, which is ignored by Git. Snapshots do not
publish releases or call providers. If this generated directory already exists,
inspect it before using `--clean`, which removes and rebuilds that directory.
On native Windows, cross-building archives does not make the workflow runnable;
run the full supported-platform checks on macOS/Linux or inside WSL.

`magent --version` prints the application version, source commit, commit timestamp
and target platform without loading configuration or starting agents. Source
builds without release linker flags report `dev` with unknown commit/date.

Future native Windows downloads require the platform work in the
[remediation plan](gap-remediation-plan.md), followed by native workflow and
provider verification. Adding `windows` to the build list alone is insufficient.

Folder and multi-repository support is available in the updated Docker preview and current source. The older v0.1.0-alpha.3 native binaries still require a single Git root.
