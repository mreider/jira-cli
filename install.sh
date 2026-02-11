#!/bin/sh
set -e

REPO="mreider/jira-cli"
INSTALL_DIR="/usr/local/bin"
BINARY="jira"

# Detect OS and architecture
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  darwin|linux) ;;
  *)
    echo "Unsupported OS: $OS"
    echo "Windows users: download from https://github.com/$REPO/releases"
    exit 1
    ;;
esac

ASSET="${BINARY}-${OS}-${ARCH}"
echo "Detected: ${OS}/${ARCH}"

# Get latest release tag
LATEST=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$LATEST" ]; then
  echo "Failed to fetch latest release from github.com/$REPO"
  exit 1
fi

URL="https://github.com/$REPO/releases/download/${LATEST}/${ASSET}"
echo "Downloading ${ASSET} ${LATEST}..."

TMP=$(mktemp)
if ! curl -fsSL "$URL" -o "$TMP"; then
  echo "Download failed: $URL"
  rm -f "$TMP"
  exit 1
fi

chmod +x "$TMP"

# Install — try directly, fall back to sudo
if [ -w "$INSTALL_DIR" ]; then
  mv "$TMP" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "$TMP" "${INSTALL_DIR}/${BINARY}"
fi

echo "Installed ${BINARY} ${LATEST} to ${INSTALL_DIR}/${BINARY}"
echo ""
echo "Run 'jira config' to set up your Atlassian credentials."
