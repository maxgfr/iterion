#!/usr/bin/env bash
#
# Install the apt prerequisites needed to compile + package
# iterion-desktop on a Debian/Ubuntu/Mint host.
#
# Usage:
#   sudo ./scripts/desktop/install-linux-deps.sh
# or:
#   ./scripts/desktop/install-linux-deps.sh           # will sudo internally
#
# Idempotent — safe to run repeatedly. Picks libfuse2t64 vs libfuse2
# (and libgtk-3-0t64 vs libgtk-3-0) automatically based on what apt
# advertises, so it works on Ubuntu 22.04, 24.04+, and Mint 21/22.

set -euo pipefail

if ! command -v apt-get >/dev/null 2>&1; then
  cat >&2 <<EOF
This script targets Debian/Ubuntu/Mint (apt-based). Other distros must
install equivalents manually:
  - GTK3 dev headers              (e.g. gtk3-devel on Fedora)
  - WebKit2GTK 4.1 dev headers    (e.g. webkit2gtk4.1-devel)
  - libsoup-3 dev headers
  - dpkg-deb, patchelf, desktop-file-utils, librsvg2
  - libfuse2 runtime              (to RUN the produced AppImage)
EOF
  exit 1
fi

# Pick the right libfuse / libgtk-3 runtime alternative for the host.
# Older Ubuntu (22.04 jammy) ship libfuse2 + libgtk-3-0; newer (24.04
# noble, Mint 22) ship the t64 ABI variants. apt-cache rejects the
# absent name, so install whichever resolves.
pick_pkg() {
  local primary="$1" fallback="$2"
  if apt-cache show "$primary" >/dev/null 2>&1; then
    echo "$primary"
  elif apt-cache show "$fallback" >/dev/null 2>&1; then
    echo "$fallback"
  else
    echo "$primary"   # let apt-get fail with a clear message
  fi
}

LIBFUSE_PKG="$(pick_pkg libfuse2t64 libfuse2)"

PKGS=(
  # WebView + windowing toolkit (build + runtime headers)
  libgtk-3-dev
  libwebkit2gtk-4.1-dev
  libsoup-3.0-dev
  # AppImage runtime + bundling
  fuse
  "$LIBFUSE_PKG"
  patchelf
  # .deb assembly + .desktop entry handling
  dpkg-dev
  desktop-file-utils
  # Icon rasterisation for the .desktop entry
  librsvg2-common
)

SUDO=""
if [ "$EUID" -ne 0 ]; then
  if ! command -v sudo >/dev/null 2>&1; then
    echo "ERROR: this script needs root (or sudo) to call apt-get." >&2
    exit 1
  fi
  SUDO="sudo"
fi

echo "Installing iterion-desktop apt prerequisites:"
printf '  %s\n' "${PKGS[@]}"
echo

$SUDO apt-get update
$SUDO apt-get install -y --no-install-recommends "${PKGS[@]}"

echo
echo "All apt prerequisites installed."
echo "Next: run 'task desktop:install-tools' to fetch the Wails CLI."
