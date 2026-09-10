#!/bin/sh
# Container setup is a delivery concern; workflow policy stays in Go.
set -eu
umask 077

fail() { printf '%s\n' "$*" >&2; exit 2; }

# Informational commands work without a workspace, volume, or provider login.
case "${1:-}" in
  --version|--help) exec magent "$@" ;;
  help)
    cat <<'EOF'
Multiharness container
  doctor [TOOL ...]     Check project, sandbox and requested tool availability
  login codex           Sign in with ChatGPT using a browser/device code
  login opencode        Configure an OpenCode provider account
  login claude          Sign in to Claude Code
  codex ARGS...         Run the bundled Codex CLI with persistent settings
  opencode ARGS...      Run the bundled OpenCode CLI with persistent settings
  claude ARGS...        Run the bundled Claude Code CLI with persistent settings
  shell                 Open a shell with the same project and tools
  [magent arguments]    Run magent (no arguments opens the interactive prompt)

Mount your project folder at /workspace and a named volume at /state.
The folder can contain multiple projects and Git repositories. No Git repository is required.
Linux: run with --user "$(id -u):$(id -g)" to preserve file ownership.
Use /login codex, /login opencode or /login claude and /config inside the interactive prompt.
Reopen the same container from any folder: docker start -ai multiharness
See https://github.com/ErOr-0/Multiharness-Core/blob/main/README.md#docker-setup
EOF
    exit 0 ;;
esac

[ "$(id -u)" != 0 ] || fail 'Run as a non-root user; on Linux use --user "$(id -u):$(id -g)".'
mountpoint -q /state || fail 'Missing persistent state: add --mount type=volume,src=magent-state,dst=/state.'
# A private home for each host UID avoids root/chown operations on user files.
HOME="/state/$(id -u)"
export HOME
[ ! -L "$HOME" ] || fail 'The persistent home must not be a symlink.'
if [ ! -e "$HOME" ]; then mkdir -m 700 "$HOME"; fi
[ -d "$HOME" ] && [ "$(stat -c %u "$HOME")" = "$(id -u)" ] && [ -w "$HOME" ] ||
  fail 'Persistent home is not owned and writable by this user; use the original UID or a new state volume.'
export CODEX_HOME="$HOME/.codex"
export XDG_CONFIG_HOME="$HOME/.config"
export XDG_DATA_HOME="$HOME/.local/share"
export XDG_STATE_HOME="$HOME/.local/state"
export XDG_CACHE_HOME="$HOME/.cache"
mkdir -p "$CODEX_HOME" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_STATE_HOME" "$XDG_CACHE_HOME"

check_workspace() {
  mountpoint -q /workspace || fail 'Mount your project folder at /workspace before running a task.'
  [ -r /workspace ] && [ -w /workspace ] && [ -x /workspace ] || fail 'The project mount must be readable and writable by this user.'

}

doctor() {
  mountpoint -q /workspace || fail 'Mount your project folder at /workspace before running a task.'
  printf 'Workspace: /workspace (mounted folder; Git optional)\nState: %s\n' "$HOME"
  for tool in codex opencode claude node npm go python3 "$@"; do
    command -v "$tool" >/dev/null 2>&1 || fail "Missing project tool: $tool. Use an image with that tool installed; host installations are separate."
  done
  printf 'Bundled tools: available. Requested tools: available.\n'
  # Offline execution of the actual sandbox, without a model or account call.
  if ! codex sandbox --config 'sandbox_mode="read-only"' -- /bin/true; then
    fail 'Codex sandbox is unavailable on this Docker host. No permissions were relaxed. See README.md#docker-setup.'
  fi
  printf 'Codex read-only sandbox: available. No model calls or project tests ran.\n'
  if codex login status >/dev/null 2>&1; then
    printf 'Codex: saved login found (account entitlement not checked).\n'
  else
    printf 'Codex: sign in using the container command: login codex\n'
  fi
  printf 'OpenCode: use opencode auth list to inspect configured providers.\n'
}

case "${1:-}" in
  doctor) shift; doctor "$@"; exit 0 ;;
  login)
    [ "$#" = 2 ] || fail 'Use login codex, login opencode or login claude.'
    case "$2" in
      codex) exec codex -c 'cli_auth_credentials_store="file"' login --device-auth ;;
      opencode) exec opencode auth login ;;
      claude) exec claude auth login ;;
      *) fail 'Use login codex, login opencode or login claude.' ;;
    esac ;;
  codex|opencode|claude) exec "$@" ;;
  shell) check_workspace; exec /bin/bash ;;

esac
check_workspace
export MAGENT_WORKSPACE_ROOT=/workspace
exec magent "$@"
