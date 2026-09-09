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
git init -q /workspace/projects/api
git init -q /tmp/untrusted
ln -s /tmp/untrusted /workspace/outside-link
cat > /tmp/startup-fixtures/magent <<'FIXTURE'
#!/bin/sh
set -eu
# The fixture must be reached without traversing the unreadable folder.
[ "$(id -u)" = 1000 ]
[ ! -r /workspace/blocked ]
[ ! -x /workspace/no-traverse ]
git -C /workspace/projects/api status --porcelain
# The scoped exception must not trust repositories elsewhere, even via symlinks.
if git -C /tmp/untrusted status --porcelain >/dev/null 2>&1; then exit 1; fi
if git -C /workspace/outside-link status --porcelain >/dev/null 2>&1; then exit 1; fi
[ ! -e "$HOME/.gitconfig" ]
printf 'PASS: unreadable child does not block startup; nested Git works; outside Git remains untrusted\n'
FIXTURE
chmod 755 /tmp/startup-fixtures/magent
entrypoint=${1:-/usr/local/bin/magent-container}
PATH="/tmp/startup-fixtures:$PATH" setpriv --reuid=1000 --regid=1000 --clear-groups /bin/sh "$entrypoint"
