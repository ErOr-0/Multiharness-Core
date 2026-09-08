#!/bin/sh
# One-time, explicit Linux host setup. Never run automatically by the launcher.
set -eu
[ "$(id -u)" -eq 0 ] || { printf 'Run with sudo on the Linux Docker host.\n' >&2; exit 2; }
[ -r /sys/module/apparmor/parameters/enabled ] &&
  [ "$(cat /sys/module/apparmor/parameters/enabled)" = Y ] || {
    printf 'AppArmor is not enabled on this host; this setup is unnecessary.\n' >&2; exit 2;
  }
command -v apparmor_parser >/dev/null 2>&1 || {
  printf 'Install your Linux distribution\047s apparmor package first.\n' >&2; exit 2;
}
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)
source_profile=$script_dir/../docker/apparmor.profile
[ -f "$source_profile" ] || { printf 'Missing docker/apparmor.profile.\n' >&2; exit 2; }
# Older parsers do not understand userns. Do not silently weaken their policy.
apparmor_parser --skip-kernel-load --skip-read-cache "$source_profile"
# Load the named profile before persisting it. No Docker defaults or sysctls change.
apparmor_parser --replace --skip-read-cache "$source_profile"
install -m 644 "$source_profile" /etc/apparmor.d/magent-container-v1
printf 'Loaded magent-container-v1 and saved it for reboot. Other container profiles are unchanged.\n'
