#!/bin/bash
set -euo pipefail

# Safebox Official Linux Installer
# Usage: curl -fsSL https://safebox.mahboob.engineer/install.sh | bash
# Pinning: curl -fsSL https://safebox.mahboob.engineer/install.sh | SAFEBOX_VERSION=v0.5.0 bash
# Fallback: curl -fsSL https://raw.githubusercontent.com/Mahboob-A/agent-safebox/main/install.sh | bash

REPO="Mahboob-A/agent-safebox"

# 1. Operating System Validation (Safebox is Linux-only)
OS="$(uname -s)"
ARCH="$(uname -m)"

if [ "$OS" != "Linux" ]; then
  echo "Error: Safebox requires Linux kernel security primitives (Landlock LSM, user namespaces, OverlayFS)." >&2
  echo "Operating system '$OS' is not supported." >&2
  exit 1
fi

# 2. Architecture Resolution
case "$ARCH" in
  x86_64|amd64)
    TARGET="linux-amd64"
    ;;
  aarch64|arm64)
    TARGET="linux-arm64"
    ;;
  *)
    echo "Error: Safebox does not support Linux architecture '$ARCH' yet." >&2
    exit 1
    ;;
esac

# 3. Version Resolution (Default: latest release)
if [ -n "${SAFEBOX_VERSION:-}" ]; then
  VERSION="$SAFEBOX_VERSION"
  TARBALL_NAME="safebox-${VERSION}-${TARGET}.tar.gz"
  DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL_NAME}"
  CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
else
  VERSION="latest"
  TARBALL_NAME="safebox-${TARGET}.tar.gz"
  DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${TARBALL_NAME}"
  CHECKSUM_URL="https://github.com/${REPO}/releases/latest/download/checksums.txt"
fi

# 4. Installation Directories
if [ "$(id -u)" -eq 0 ]; then
  BIN_HOME="/usr/local/bin"
else
  BIN_HOME="${XDG_BIN_HOME:-$HOME/.local/bin}"
fi

STATE_HOME="${XDG_STATE_HOME:-$HOME/.local/state}/safebox"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo "========================================================"
echo " Installing Safebox (${VERSION}, ${TARGET})"
echo "========================================================"

# 5. Download Release Tarball
TARBALL="$TMP/$TARBALL_NAME"
echo "Downloading from GitHub Releases..."
if ! curl -fL --retry 3 --retry-delay 2 --progress-bar "$DOWNLOAD_URL" -o "$TARBALL"; then
  echo "Error: Failed to download release tarball from $DOWNLOAD_URL" >&2
  echo "Please verify that the release exists on https://github.com/${REPO}/releases" >&2
  exit 1
fi

# 6. Checksum Verification (if checksum file is present)
CHECKSUM_FILE="$TMP/checksums.txt"
if curl -fsSL "$CHECKSUM_URL" -o "$CHECKSUM_FILE" 2>/dev/null; then
  echo "Verifying SHA256 checksum..."
  EXPECTED_SHA="$(grep -E "(^|[[:space:]])${TARBALL_NAME}$" "$CHECKSUM_FILE" | awk '{print $1}' | head -n 1 || true)"
  if [ -n "$EXPECTED_SHA" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      ACTUAL_SHA="$(sha256sum "$TARBALL" | awk '{print $1}')"
    else
      ACTUAL_SHA="$(shasum -a 256 "$TARBALL" | awk '{print $1}')"
    fi
    if [ "$EXPECTED_SHA" != "$ACTUAL_SHA" ]; then
      echo "Error: Downloaded checksum does not match official release!" >&2
      echo "Expected: $EXPECTED_SHA" >&2
      echo "Actual:   $ACTUAL_SHA" >&2
      exit 1
    fi
    echo "Checksum verified."
  fi
fi

# 7. Unpack and Atomic Install
echo "Unpacking binary..."
tar -xzf "$TARBALL" -C "$TMP"

BIN_SRC="$TMP/safebox-${TARGET}"
if [ ! -f "$BIN_SRC" ]; then
  if [ -f "$TMP/safebox" ]; then
    BIN_SRC="$TMP/safebox"
  else
    echo "Error: Release tarball did not contain expected binary." >&2
    exit 1
  fi
fi

mkdir -p "$BIN_HOME"
TARGET_BIN="$BIN_HOME/safebox"
TARGET_TMP="$BIN_HOME/safebox.tmp.$$"

cp "$BIN_SRC" "$TARGET_TMP"
chmod 755 "$TARGET_TMP"
mv -f "$TARGET_TMP" "$TARGET_BIN"

# 8. Write Installation Metadata
mkdir -p "$STATE_HOME"
cat > "$STATE_HOME/install.json" <<EOF
{
  "version": "$VERSION",
  "target": "$TARGET",
  "binary": "$TARGET_BIN",
  "installed_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF

echo "========================================================"
echo " Safebox successfully installed to: $TARGET_BIN"
echo "========================================================"

# 9. Verify Shell PATH
case ":$PATH:" in
  *":$BIN_HOME:"*)
    echo
    echo "Run 'safebox help' or 'safebox version' to get started."
    ;;
  *)
    echo
    echo "Notice: $BIN_HOME is not currently in your PATH."
    echo "Add it to your shell configuration to run 'safebox' from anywhere:"
    case "${SHELL:-}" in
      */zsh)  echo "  echo 'export PATH="$BIN_HOME:\$PATH"' >> ~/.zshrc && exec zsh" ;;
      */bash) echo "  echo 'export PATH="$BIN_HOME:\$PATH"' >> ~/.bashrc && exec bash" ;;
      *)      echo "  export PATH="$BIN_HOME:\$PATH" (add to your shell rc file)" ;;
    esac
    ;;
esac
