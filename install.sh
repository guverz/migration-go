#!/usr/bin/env bash

set -euo pipefail

REPO="guverz/migration-go"
BINARY="migration-go"

VERSION=$(curl -fsSL \
  "https://api.github.com/repos/${REPO}/releases/latest" |
  grep '"tag_name"' |
  sed -E 's/.*"([^"]+)".*/\1/')

OS="$(uname | tr '[:upper:]' '[:lower:]')"

ARCH="$(uname -m)"

case "$ARCH" in
  x86_64)
    ARCH="amd64"
    ;;
  aarch64|arm64)
    ARCH="arm64"
    ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

FILE="migration-go_${VERSION#v}_${OS}_${ARCH}.zip"

URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILE}"

TMP=$(mktemp -d)

curl -fL "$URL" -o "$TMP/$FILE"

cd "$TMP"

unzip "$FILE"

chmod +x migration-go

sudo mv migration-go /usr/local/bin/

echo "Installed migration-go"