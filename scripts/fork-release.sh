#!/usr/bin/env bash
# Build the upgrade package for a fork release.
#
# The panel's update check only looks at this fork's GitHub Releases
# (core/app/service/fork_release.go). A new version is offered to panels
# ONLY after you publish a release here, so the flow is:
#
#   1. bash scripts/fork-release.sh v2.1.5-cluster.2 /path/to/upstream-1panel-vX-linux-amd64.tar.gz amd64
#   2. Create a GitHub release on brlanweb/1Panel with tag v2.1.5-cluster.2
#      (mark it "pre-release" to offer it only to panels in developer mode),
#      write the release note in the body, and upload
#      build/release/1panel-v2.1.5-cluster.2-linux-amd64.tar.gz as an asset.
#   3. Panels running an older version see the update within ~10 minutes.
#
# The upstream package is only used for the files this repo does not
# build (1pctl, initscript/, lang/, GeoIP.mmdb, install.sh); both binaries
# are replaced by this fork's build and ORIGINAL_VERSION is set to the tag.
set -euo pipefail

TAG="${1:?usage: fork-release.sh <tag> <upstream-package.tar.gz> [amd64|arm64]}"
BASE_PKG="${2:?usage: fork-release.sh <tag> <upstream-package.tar.gz> [amd64|arm64]}"
ARCH="${3:-amd64}"

case "$TAG" in
v2.*) ;;
*) echo "tag must start with v2. (it becomes the panel version)" >&2; exit 1 ;;
esac

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/build/release"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

echo ">> building frontend"
(cd "$ROOT/frontend" && npm ci && npm run build:pro)

echo ">> building core / agent for linux/$ARCH"
bash "$ROOT/agent/utils/vm/helper/prepare.sh"
(cd "$ROOT/core" && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags '-s -w' -o "$WORK/1panel-core" ./cmd/server/main.go)
(cd "$ROOT/agent" && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -trimpath -ldflags '-s -w' -o "$WORK/1panel-agent" ./cmd/server/main.go)

echo ">> repacking $BASE_PKG"
tar -xzf "$BASE_PKG" -C "$WORK"
SRC_DIR="$(find "$WORK" -mindepth 1 -maxdepth 1 -type d -name '1panel-*' | head -n1)"
[ -n "$SRC_DIR" ] || { echo "upstream package has no 1panel-* directory" >&2; exit 1; }
PKG_NAME="1panel-$TAG-linux-$ARCH"
PKG_DIR="$WORK/$PKG_NAME"
[ "$SRC_DIR" = "$PKG_DIR" ] || mv "$SRC_DIR" "$PKG_DIR"

for f in 1pctl initscript lang GeoIP.mmdb; do
	[ -e "$PKG_DIR/$f" ] || { echo "upstream package is missing $f" >&2; exit 1; }
done
install -m 755 "$WORK/1panel-core" "$PKG_DIR/1panel-core"
install -m 755 "$WORK/1panel-agent" "$PKG_DIR/1panel-agent"
sed -i.bak "s/^ORIGINAL_VERSION=.*/ORIGINAL_VERSION=$TAG/" "$PKG_DIR/1pctl" && rm -f "$PKG_DIR/1pctl.bak"

mkdir -p "$OUT"
tar -czf "$OUT/$PKG_NAME.tar.gz" -C "$WORK" "$PKG_NAME"
(cd "$OUT" && shasum -a 256 "$PKG_NAME.tar.gz" > "$PKG_NAME.tar.gz.sha256")
echo ">> done: $OUT/$PKG_NAME.tar.gz"
echo "   upload it to the GitHub release with tag $TAG"
