#!/bin/sh
# Run in the built image with this repository mounted read-only at /source.
set -eu
export HOME=/tmp/magent-check-home
export GOTOOLCHAIN=local
export MULTIHARNESS_SMOKE=0 MULTIHARNESS_SMOKE_FALLBACK=0
export MULTIHARNESS_RUNTIME_CHECK=0 MULTIHARNESS_INSTALL_MODE=disabled
mkdir -p "$HOME" /tmp/magent-check-source
tar -C /source --exclude=.git --exclude=.coverage --exclude=dist -cf - . |
  tar -C /tmp/magent-check-source -xf -
cd /tmp/magent-check-source
git init -q
git add .
make fmt
make check
make lint-workflows
