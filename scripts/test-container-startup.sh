#!/bin/sh
# Disposable-container test only: /workspace and /state must be empty tmpfs mounts.
set -eu
[ "$(id -u)" = 0 ]
mountpoint -q /workspace
mountpoint -q /state
mkdir -p /workspace/projects/api /workspace/blocked /tmp/startup-fixtures /tmp/untrusted
mkdir /workspace/no-traverse
chmod 000 /workspace/blocked
chmod 444 /workspace/no-traverse
mkdir /workspace/projects/api/.git
printf broken > /workspace/projects/api/.git/HEAD
ln -s /tmp/untrusted /workspace/outside-link
cat > /tmp/startup-fixtures/magent <<'FIXTURE'
#!/bin/sh
set -eu
# The fixture must be reached without traversing the unreadable folder.
[ "$(id -u)" = 1000 ]
[ ! -r /workspace/blocked ]
[ ! -x /workspace/no-traverse ]
[ -z "${GIT_CONFIG_SYSTEM:-}" ]
[ ! -e "$HOME/.gitconfig" ]
printf 'PASS: unreadable child does not block startup; no Git inspection or configuration\n'
FIXTURE
cat > /tmp/startup-fixtures/git <<'FIXTURE'
#!/bin/sh
printf 'Git must not be invoked during startup\n' >&2
exit 99
FIXTURE
chmod 755 /tmp/startup-fixtures/magent /tmp/startup-fixtures/git
entrypoint=${1:-/usr/local/bin/magent-container}
PATH="/tmp/startup-fixtures:$PATH" setpriv --reuid=1000 --regid=1000 --clear-groups /bin/sh "$entrypoint"
