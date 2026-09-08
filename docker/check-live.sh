#!/bin/sh
# Explicitly opted-in, bounded provider workflows using existing container auth.
set -eu
[ "${MULTIHARNESS_SMOKE:-}" = 1 ] || { printf 'Set MULTIHARNESS_SMOKE=1 to authorize live provider calls.\n' >&2; exit 2; }
[ -z "${CI:-}" ] || { printf 'Live credentials must stay out of CI.\n' >&2; exit 2; }
[ "$#" -eq 1 ] && [ -n "$1" ] || { printf 'Supply one explicit implementation model; select its harness in MULTIHARNESS_SMOKE_CONFIG.\n' >&2; exit 2; }
[ -d "${HOME:?}/.codex" ] || { printf 'Use your existing private container home and authenticate first.\n' >&2; exit 2; }
source_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
scratch=$(mktemp -d /tmp/magent-live-XXXXXX)
trap 'rm -rf -- "$scratch"' EXIT HUP INT TERM
tar -C "$source_dir" --exclude=.git --exclude=.coverage --exclude=dist --exclude=node_modules -cf - . | tar -C "$scratch" -xf -
cd "$scratch"
# The smoke harness deliberately rejects inherited Git overrides. Its own
# repositories are newly created by this user and need no host ownership overlay.
unset GIT_CONFIG_SYSTEM
export MULTIHARNESS_SMOKE_MODEL="$1" MULTIHARNESS_INSTALL_MODE=disabled GOTOOLCHAIN=local
go test -count=1 -timeout 45m -v ./cmd/multiharness -run '^(TestSmokeWorkflow|TestSmokePlainFolderAnswer|TestSmokeAgentCancellation)$'
