#!/usr/bin/env bash
# Produce the vendored source tarball the Flathub manifest builds from:
# the full source tree plus vendor/ (Go modules) and frontend/dist (built
# frontend), so the sandboxed Flathub build needs no network.
#
#   ./make-vendored-tarball.sh 0.1.0
#
# Writes novelide-<version>-vendored.tar.xz and its .sha256 next to this
# script. Used by the release workflow; also runnable locally.
set -euo pipefail

VERSION="${1:?usage: make-vendored-tarball.sh <version>}"
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
NAME="novelide-$VERSION-vendored"
OUT="$HERE/$NAME.tar.xz"

echo "==> frontend build"
(cd "$ROOT/frontend" && { [ -d node_modules ] || npm ci || npm install; } && npm run build)
echo "==> go mod vendor"
(cd "$ROOT" && go mod vendor)

# Stamp the version into the embedded version.txt so the packed tarball is
# self-describing (the app reports its real version with no build-time env).
# The AppStream changelog is injected separately by the release workflow, which
# pulls it live from the GitHub release notes. Restored afterward so a local
# run doesn't leave the working tree dirty.
echo "==> stamping version $VERSION"
printf '%s\n' "$VERSION" > "$ROOT/version.txt"
trap 'git -C "$ROOT" checkout -- version.txt 2>/dev/null || true' EXIT

echo "==> packing $OUT"
tar -C "$ROOT" \
  --exclude='./build/bin' \
  --exclude='./build/flatpak/.flatpak-build' \
  --exclude='./build/flatpak/.flatpak-repo' \
  --exclude='./build/flatpak/.flatpak-builder' \
  --exclude='./build/flatpak/novelide.flatpak' \
  --exclude='./build/flatpak/'"$NAME"'.tar.xz' \
  --exclude='./frontend/node_modules' \
  --exclude='./.git' \
  --transform "s,^\./,$NAME/," \
  -cJf "$OUT" .

sha256sum "$OUT" | tee "$OUT.sha256"
echo "==> paste that sha256 into build/flatpak/flathub/dev.kilgore.NovelIDE.yml"
