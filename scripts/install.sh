#!/bin/bash
set -euo pipefail

REPO="struckchure/axel"
LATEST_RELEASE_API="https://api.github.com/repos/${REPO}/releases/latest"

# ── Cleanup on exit ────────────────────────────────────────────────────────────
TMP_DIR=""
cleanup() {
    if [ -n "$TMP_DIR" ] && [ -d "$TMP_DIR" ]; then
        rm -rf "$TMP_DIR"
    fi
}
trap cleanup EXIT

# ── Version resolution ─────────────────────────────────────────────────────────
resolve_latest_version() {
    local v
    v=$(curl -fsSL "$LATEST_RELEASE_API" \
        | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' \
        | head -n 1)
    if [ -z "$v" ]; then
        echo "error: could not resolve latest version from GitHub API" >&2
        exit 1
    fi
    echo "$v"
}

if [ -n "${1:-}" ]; then
    VERSION="$1"
else
    VERSION=$(resolve_latest_version)
fi

echo "Installing axel ${VERSION}..."

# ── Platform detection ─────────────────────────────────────────────────────────
OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
    Linux)
        case "$ARCH" in
            x86_64)  FILE="axel_Linux_x86_64.tar.gz" ;;
            aarch64) FILE="axel_Linux_arm64.tar.gz"  ;;
            *)
                echo "error: unsupported architecture: $ARCH" >&2
                exit 1
                ;;
        esac
        ;;
    Darwin)
        case "$ARCH" in
            x86_64) FILE="axel_Darwin_x86_64.tar.gz" ;;
            arm64)  FILE="axel_Darwin_arm64.tar.gz"  ;;
            *)
                echo "error: unsupported architecture: $ARCH" >&2
                exit 1
                ;;
        esac
        ;;
    *)
        echo "error: unsupported OS: $OS" >&2
        exit 1
        ;;
esac

# ── Download & extract ─────────────────────────────────────────────────────────
DEST_DIR="$HOME/.axel/bin"
mkdir -p "$DEST_DIR"

TMP_DIR=$(mktemp -d)
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILE}"

echo "Downloading ${FILE}..."
if ! curl -fsSL "$DOWNLOAD_URL" -o "$TMP_DIR/$FILE"; then
    echo "error: failed to download $DOWNLOAD_URL" >&2
    exit 1
fi

echo "Extracting to $DEST_DIR..."
if ! tar -xzf "$TMP_DIR/$FILE" -C "$DEST_DIR"; then
    echo "error: failed to extract $FILE" >&2
    exit 1
fi

# ── PATH update ────────────────────────────────────────────────────────────────
PATH_LINE="export PATH=\"\$HOME/.axel/bin:\$PATH\""

# Determine which shell profile to update.
if [ -n "${ZSH_VERSION:-}" ] || [ "$(basename "${SHELL:-}")" = "zsh" ]; then
    PROFILE="$HOME/.zshrc"
elif [ -f /etc/alpine-release ]; then
    PROFILE="$HOME/.profile"
else
    PROFILE="$HOME/.bashrc"
fi

if ! grep -Fq '.axel/bin' "$PROFILE" 2>/dev/null; then
    echo "" >> "$PROFILE"
    echo "# Added by axel installer" >> "$PROFILE"
    echo "$PATH_LINE" >> "$PROFILE"
    echo "Updated $PROFILE"
fi

# ── Done ───────────────────────────────────────────────────────────────────────
echo ""
echo "axel ${VERSION} installed to $DEST_DIR/axel"
echo ""
echo "Restart your terminal or run:"
echo "  source $PROFILE"
echo ""
echo "Then verify with:"
echo "  axel version"
