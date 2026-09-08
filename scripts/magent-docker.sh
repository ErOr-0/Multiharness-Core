#!/bin/sh
# Run from any project; no host Git, Go, Node, or provider CLI is required.
set -eu
project=$PWD
image=${MAGENT_IMAGE:-er0r2/multiharness-core:preview}
state=${MAGENT_STATE_VOLUME:-magent-state}
tty=yes
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
seccomp=$script_dir/../docker/seccomp.json
[ -f "$seccomp" ] || { printf 'Missing docker/seccomp.json beside the launcher package.\n' >&2; exit 2; }
while [ "$#" -gt 0 ]; do
  case "$1" in
    --project|--image|--state)
      [ "$#" -ge 2 ] || { printf 'Missing value for %s\n' "$1" >&2; exit 2; }
      case "$1" in --project) project=$2 ;; --image) image=$2 ;; --state) state=$2 ;; esac
      shift 2 ;;
    --no-tty) tty=no; shift ;;
    --) shift; break ;;
    *) break ;;
  esac
done
project=$(cd "$project" && pwd -P)
case "$project" in *','*|*'"'*) printf 'Docker project paths containing commas or quotes are unsupported by this launcher.\n' >&2; exit 2 ;; esac
case "$image" in ''|-*) printf 'Invalid image name.\n' >&2; exit 2 ;; esac
case "$state" in ''|*[!a-zA-Z0-9_.-]*|-*|.*|_*) printf 'Use an alphanumeric Docker volume name.\n' >&2; exit 2 ;; esac
if [ "$tty" = yes ] && [ -t 0 ] && [ -t 1 ]; then set -- --tty "$image" "$@"; else set -- "$image" "$@"; fi
case "$(uname -s)" in
  Linux) set -- --user "$(id -u):$(id -g)" "$@" ;;
  Darwin) ;;
  *) printf 'On Windows, use magent-docker.ps1 from PowerShell.\n' >&2; exit 2 ;;
esac
exec docker run --rm --init --interactive \
  --cap-drop ALL --security-opt no-new-privileges=true --security-opt "seccomp=$seccomp" \
  --mount "type=bind,src=$project,dst=/workspace" \
  --mount "type=volume,src=$state,dst=/state" "$@"
