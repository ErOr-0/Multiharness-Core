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
ARG CLAUDE_VERSION=2.1.267
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git openssh-client ripgrep bubblewrap util-linux \
    make gcc g++ libc6-dev python3 curl \
    && rm -rf /var/lib/apt/lists/* \
    && npm install --global --no-audit --no-fund \
       "@openai/codex@${CODEX_VERSION}" "opencode-ai@${OPENCODE_VERSION}" "@anthropic-ai/claude-code@${CLAUDE_VERSION}" \
    && npm cache clean --force \
    && codex --version && opencode --version && claude --version \
    && install -d -m 1777 /state \
    && install -d -m 755 /workspace \
    && git config --system --add safe.directory /workspace
# Official, immutable Muse Code 1.3.0 release; checksums from Meta's release
# manifest. No subscription credentials or mutable installer execute at build.
RUN case "$TARGETARCH" in \
      amd64) muse_platform=x86; muse_sha=71b089d055dfe6e4562092bc484896b61bd96fd6ef9fef9da54a14aa174e2a33 ;; \
      arm64) muse_platform=aarch64; muse_sha=5e5ea2a3de3a3fabdff8982aec9423d20eaa7dad05df37efb4264356d0d2e223 ;; \
      *) exit 1 ;; \
    esac \
    && curl --fail --location --retry 3 --proto '=https' --tlsv1.2 \
      "https://lookaside.facebook.com/lookaside/muse/download/?channel=muse&version=1.3.0-R3401.1&file=muse-${muse_platform}-linux" \
      --output /usr/local/bin/muse \
    && echo "$muse_sha  /usr/local/bin/muse" | sha256sum --check --strict \
    && chmod 755 /usr/local/bin/muse \
    && muse --version
COPY --from=go-toolchain /usr/local/go /usr/local/go
# Agent commands often use bash -lc, whose /etc/profile replaces PATH. Keep the
# bundled Go tools available through the standard login-shell executable path.
RUN ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt
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
