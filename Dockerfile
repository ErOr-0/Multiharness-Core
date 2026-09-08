# syntax=docker/dockerfile:1
ARG GO_IMAGE=golang:1.26.6-bookworm@sha256:116d58cbd88c1297624acc6e967a060012422bacf9930927e23fb719189c6f36
ARG NODE_IMAGE=node:24-bookworm-slim@sha256:ba849c60be29959425b8734d57b8b4b7d56f98edd9504c9af091d5281095a71e

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${BUILD_DATE}" \
    -o /out/magent ./cmd/multiharness

FROM ${GO_IMAGE} AS go-toolchain
FROM ${NODE_IMAGE} AS runtime
# Keep agent versions aligned with internal/adapter/setup/install.go.
ARG CODEX_VERSION=0.153.0
ARG OPENCODE_VERSION=1.18.23
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git openssh-client ripgrep bubblewrap util-linux \
    make gcc g++ libc6-dev python3 curl \
    && rm -rf /var/lib/apt/lists/* \
    && npm install --global --no-audit --no-fund \
       "@openai/codex@${CODEX_VERSION}" "opencode-ai@${OPENCODE_VERSION}" \
    && npm cache clean --force \
    && codex --version && opencode --version \
    && install -d -m 1777 /state \
    && install -d -m 755 /workspace \
    && git config --system --add safe.directory /workspace
COPY --from=go-toolchain /usr/local/go /usr/local/go
COPY --from=build /out/magent /usr/local/bin/magent
COPY --chmod=755 docker/entrypoint.sh /usr/local/bin/magent-container
ENV PATH="/usr/local/go/bin:${PATH}" \
    MULTIHARNESS_INSTALL_MODE=disabled
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown
LABEL org.opencontainers.image.title="Multiharness Core" \
      org.opencontainers.image.source="https://github.com/ErOr-0/Multiharness-Core" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.description="Local Codex/OpenCode workflow with persistent provider login and a mounted project folder; Git optional"
USER 1000:1000
WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/magent-container"]
CMD []
