#!/bin/sh
# Installs harness-bridge from the latest GitHub release, then starts its setup.
#
#   curl -fsSL https://raw.githubusercontent.com/ErOr-0/Multiharness-Core/main/install.sh | sh
#
# HARNESS_BRIDGE_VERSION    a release tag such as v0.1.0 (default: latest)
# HARNESS_BRIDGE_INSTALL_DIR where to put it (default: ~/.local/bin)
# HARNESS_BRIDGE_NO_SETUP   set to skip starting setup afterwards
set -eu

repo="ErOr-0/Multiharness-Core"
version="${HARNESS_BRIDGE_VERSION:-latest}"
dest="${HARNESS_BRIDGE_INSTALL_DIR:-$HOME/.local/bin}"

fail() {
	echo "harness-bridge: $*" >&2
	exit 1
}

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
linux | darwin) ;;
*) fail "this script supports macOS and Linux; on Windows use install.ps1" ;;
esac
case "$(uname -m)" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) fail "unsupported CPU $(uname -m)" ;;
esac

if [ "$version" = latest ]; then
	base="https://github.com/$repo/releases/latest/download"
else
	base="https://github.com/$repo/releases/download/$version"
fi
base="${HARNESS_BRIDGE_DOWNLOAD_BASE:-$base}"
asset="harness-bridge_${os}_${arch}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
echo "Downloading harness-bridge for $os/$arch..."
download() {
	curl -fsSL --retry 2 "$1" -o "$2" ||
		fail "couldn't download $1. Check that https://github.com/$repo/releases has a release and that you're online, then run this again."
}
download "$base/$asset" "$tmp/$asset"
download "$base/checksums.txt" "$tmp/checksums.txt"

expected=$(awk -v f="$asset" '{ n = $2; sub(/^\*/, "", n); if (n == f) print $1 }' "$tmp/checksums.txt")
[ -n "$expected" ] || fail "checksums.txt has no entry for $asset"
if command -v sha256sum >/dev/null 2>&1; then
	actual=$(sha256sum "$tmp/$asset" | awk '{ print $1 }')
else
	actual=$(shasum -a 256 "$tmp/$asset" | awk '{ print $1 }')
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for $asset; nothing was installed"

mkdir -p "$dest"
cp "$tmp/$asset" "$dest/harness-bridge.new"
chmod 755 "$dest/harness-bridge.new"
mv -f "$dest/harness-bridge.new" "$dest/harness-bridge"
echo "Installed $dest/harness-bridge"
case ":$PATH:" in
*":$dest:"*) ;;
*) echo "To run it by name, add $dest to PATH in your shell profile. Agents don't need this." ;;
esac

if [ -n "${HARNESS_BRIDGE_NO_SETUP:-}" ]; then
	echo "Run harness-bridge to set it up."
elif [ -r /dev/tty ]; then
	# curl | sh gives this script the pipe as stdin; answer from the terminal.
	"$dest/harness-bridge" setup </dev/tty
else
	echo "No terminal here; run harness-bridge in a terminal to set it up."
fi
