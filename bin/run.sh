#!/usr/bin/env bash
set -euo pipefail

DIR="$(cd "$(dirname "$0")" && pwd)"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
esac

BIN="$DIR/cc-usage-${OS}-${ARCH}"
if [ "$OS" = "windows_nt" ] || [ "${OS%%_*}" = "mingw64" ] || [ "${OS%%_*}" = "msys" ]; then
  BIN="$DIR/cc-usage-windows-amd64.exe"
fi

if [ ! -x "$BIN" ]; then
  echo "cc-usage: no binary for ${OS}/${ARCH}" >&2
  exit 1
fi

exec "$BIN" "$@"
