#!/usr/bin/env bash
# Build NovelIDE as a flatpak, FROM SOURCE inside the sandbox — the same
# build process Flathub uses.
#
#   ./build-flatpak.sh            build + install into the user installation
#   ./build-flatpak.sh --bundle   also produce novelide.flatpak for sharing
#
# Requires: flatpak, node/npm, go. flatpak-builder is installed as a
# flatpak automatically if missing.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
APP_ID=dev.kilgore.NovelIDE
MANIFEST="$HERE/$APP_ID.yml"
BUILD_DIR="$HERE/.flatpak-build"
REPO_DIR="$HERE/.flatpak-repo"
GNOME_VERSION=50
GO_EXT_BRANCH=25.08 # freedesktop base of the GNOME runtime above

# 1. Prepare the offline inputs the sandbox build needs.
echo "==> frontend build"
(cd "$ROOT/frontend" && { [ -d node_modules ] || npm ci || npm install; } && npm run build)
echo "==> go mod vendor"
(cd "$ROOT" && go mod vendor)

# 2. Find a flatpak-builder.
if command -v flatpak-builder >/dev/null; then
  BUILDER=(flatpak-builder)
elif flatpak info org.flatpak.Builder >/dev/null 2>&1; then
  BUILDER=(flatpak run org.flatpak.Builder)
else
  echo "==> installing org.flatpak.Builder (one-time)"
  flatpak install -y --user flathub org.flatpak.Builder
  BUILDER=(flatpak run org.flatpak.Builder)
fi

# 3. Make sure the runtime, SDK, and Go extension exist.
flatpak install -y --user --noninteractive flathub \
  "org.gnome.Platform//$GNOME_VERSION" "org.gnome.Sdk//$GNOME_VERSION" \
  "org.freedesktop.Sdk.Extension.golang//$GO_EXT_BRANCH" 2>/dev/null || true

# 4. Build + install into the user installation.
echo "==> flatpak-builder (compiling inside the sandbox — first run takes a while)"
"${BUILDER[@]}" --user --install --force-clean \
  --repo="$REPO_DIR" "$BUILD_DIR" "$MANIFEST"

echo "==> installed. Run with: flatpak run $APP_ID"

# 5. Optional single-file bundle.
if [[ "${1:-}" == "--bundle" ]]; then
  echo "==> building bundle"
  flatpak build-bundle "$REPO_DIR" "$HERE/novelide.flatpak" "$APP_ID"
  echo "==> bundle at build/flatpak/novelide.flatpak"
fi
