#!/usr/bin/env bash
# One-time interactive Docker setup. Does not install a host command.
set -euo pipefail
umask 077
setup_root="$(cd -- "$(dirname -- "$0")/.." && pwd -P)"
command -v docker >/dev/null || { echo 'Install and open Docker, then run setup again.' >&2; exit 1; }
docker info >/dev/null
docker compose version >/dev/null

if [ ! -f "$setup_root/.env" ]; then
  printf 'First-time setup. Choose the host folder that contains your projects.\n'
  read -r -p 'Full folder path (no surrounding quotes): ' workspace_folder
  case "$workspace_folder" in
    '~') workspace_folder="$HOME" ;;
    '~/'*) workspace_folder="$HOME/${workspace_folder:2}" ;;
  esac
  [ -d "$workspace_folder" ] || { echo 'That folder does not exist. Run setup again with an existing folder.' >&2; exit 1; }
  workspace_folder="$(cd -- "$workspace_folder" && pwd -P)"
  case "$workspace_folder" in *$'\n'*|*$'\r'*) echo 'Folder paths containing line breaks are unsupported.' >&2; exit 1 ;; esac
  setup_uid=1000
  setup_gid=1000
  if [ "$(uname -s)" = Linux ]; then
    setup_uid="$(id -u)"
    setup_gid="$(id -g)"
    [ "$setup_uid" != 0 ] || { echo 'Run setup as your regular user, not root.' >&2; exit 1; }
  fi
  # Compose single-quoted values preserve spaces and dollar signs literally.
  quote_replacement="\\'"
  workspace_folder="${workspace_folder//\'/$quote_replacement}"
  setup_tmp="$(mktemp "$setup_root/.env.XXXXXX")"
  trap 'rm -f "$setup_tmp"' EXIT
  printf "MULTIHARNESS_WORKSPACE='%s'\nMULTIHARNESS_UID=%s\nMULTIHARNESS_GID=%s\n" "$workspace_folder" "$setup_uid" "$setup_gid" > "$setup_tmp"
  mv "$setup_tmp" "$setup_root/.env"
  trap - EXIT
  printf 'Folder saved. You will choose a project and configure your team inside the app.\n'
else
  printf 'Using your saved setup.\n'
fi

if [ "$(docker inspect --format '{{.State.Running}}' multiharness 2>/dev/null || true)" = true ]; then
  printf 'Multiharness is already running. Reconnect with: docker attach multiharness\n'
  exit 0
fi
compose_args=(--env-file "$setup_root/.env" -f "$setup_root/compose.yaml")
if [ "$(uname -s)" = Linux ] && docker info --format '{{json .SecurityOptions}}' | grep -q name=apparmor; then
  printf 'Installing the scoped AppArmor policy for this Linux host.\n'
  sudo sh "$setup_root/scripts/magent-apparmor.sh"
  compose_args+=(-f "$setup_root/docker/compose.linux.yaml")
fi
docker compose "${compose_args[@]}" pull
docker compose "${compose_args[@]}" up --no-start
printf 'Later, start from any folder with: docker start -ai multiharness\n'
exec docker start -ai multiharness
