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

CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

TMP=$(mktemp -d)

curl -fL "$URL" -o "$TMP/$FILE"

curl -fL "$CHECKSUM_URL" -o "$TMP/checksums.txt"

EXPECTED=$(grep " $FILE$" "$TMP/checksums.txt" | awk '{print $1}')

if [ -z "$EXPECTED" ]; then
    echo "Could not find checksum for $FILE"
    exit 1
fi

ACTUAL=$(sha256sum "$TMP/$FILE" | awk '{print $1}')

if [ "$EXPECTED" != "$ACTUAL" ]; then
    echo "Checksum verification failed"
    exit 1
fi

echo "Checksum verified"

cd "$TMP"

unzip "$FILE"

chmod +x migration-go

sudo mv migration-go /usr/local/bin/

echo "Installed migration-go"