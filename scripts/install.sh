#!/bin/sh

set -eu

REPO="querysheriff/collector"
SERVICE="querysheriff-collector"

fail() { echo "install: $1" >&2; exit 1; }

[ "$(id -u)" = 0 ] || fail "must run as root (try: sudo sh)"
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v dpkg >/dev/null 2>&1 || fail "this installer supports Debian-based systems only"

if [ -r /etc/os-release ]; then
  # shellcheck source=/dev/null
  os_id=$(. /etc/os-release; echo "${ID:-}")
  os_version_id=$(. /etc/os-release; echo "${VERSION_ID:-}")
  os_pretty=$(. /etc/os-release; echo "${PRETTY_NAME:-unknown}")
  case "$os_id:$os_version_id" in
    debian:11|debian:12|debian:13) : ;;
    *) echo "install: warning: tested on Debian 11-13, found $os_pretty" >&2 ;;
  esac
fi

ARCH="$(dpkg --print-architecture)"
[ "$ARCH" = amd64 ] || fail "unsupported architecture $ARCH (only amd64 is published)"

VERSION="${VERSION:-latest}"
if [ "$VERSION" = latest ]; then
  TAG=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
        | grep -m1 '"tag_name"' | cut -d'"' -f4)
  [ -n "$TAG" ] || fail "could not resolve the latest release tag"
else
  TAG="v${VERSION#v}"
fi
VER="${TAG#v}"

DEB="querysheriff-collector_${VER}_${ARCH}.deb"
URL="https://github.com/$REPO/releases/download/${TAG}/${DEB}"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

echo "install: downloading $DEB ($TAG)"
curl -fsSL "$URL" -o "$TMP/$DEB" || fail "download failed: $URL"

echo "install: installing package"
if command -v apt-get >/dev/null 2>&1; then
  apt-get install -y "$TMP/$DEB" || { dpkg -i "$TMP/$DEB" || true; apt-get -f install -y; }
else
  dpkg -i "$TMP/$DEB"
fi

cat <<EOF

querysheriff-collector installed.

Next steps:
  1. Edit the config:   /etc/querysheriff-collector.yml
  2. Start the service: systemctl start $SERVICE
  3. Check it:          systemctl status $SERVICE
                        journalctl -u $SERVICE -f

EOF
