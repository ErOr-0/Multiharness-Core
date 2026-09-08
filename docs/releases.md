# Docker releases

The canonical download is `er0r2/multiharness:latest`. Immutable version tags use
the same image index for linux/amd64 and linux/arm64. Docker selects the host
architecture. The image remains preview quality until the documented live gates
are complete; `latest` names the current download, not a stability guarantee.

Follow [Docker setup](docker.md). The single configuration bundle contains
Compose, small interactive setup scripts and the required platform sandbox policies. There
are no host launcher/native executable downloads in the supported installation.

For local development, `make build-dev` produces `dist/multiharness-dev`; it does
not replace a user's command in PATH. Developers provide their own selected
agent CLIs and use the [CLI reference](cli.md).

## Publish

Use the Docker image workflow on a reviewed source commit. It builds from the
canonical Dockerfile, runs native architecture/container checks, pushes an
immutable version tag, then updates `latest` to that checked index. Its path
filters include runtime code as well as Docker files. Verify both registry
platforms and record the source commit, image digest and actual test evidence.
The optional publishing job requires `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`
in the GitHub `dockerhub` environment. Without those credentials, run the native
CI checks on the recorded source commit, then build and publish from that clean
commit using an already authenticated maintainer Docker account. Do not publish
uncommitted local binaries.

`make package-docker` assembles `dist/multiharness-docker.zip`. The website build
uses the same packaging script, so it serves the maintained Compose and policy
files rather than generated copies or per-platform executables. Publish the
website and Docker Hub description with the matching image and guide.

Existing users stop and recreate the named container with their saved Compose
file to apply an image update. Retain the `magent-state` volume and host bind.
See [release-readiness.md](release-readiness.md) for historical and pending gates.
