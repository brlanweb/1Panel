#!/usr/bin/env bash
# One-command installer for the brlanweb/1Panel fork.
#
#   bash <(curl -fsSL https://github.com/brlanweb/1Panel/releases/latest/download/quick_start.sh)
#
# Options are passed straight to the package's install.sh, e.g.
#   bash <(curl -fsSL .../quick_start.sh) -y --lang zh --port 10086 --entrance mypanel --username admin
#
# Environment:
#   PANEL_VERSION   install a specific tag (default: latest release)
#   GITHUB_PROXY    prefix for github.com downloads, e.g. https://ghfast.top/
#   PANEL_*         any variable understood by install.sh (PANEL_PASSWORD, ...)
set -euo pipefail

REPO="brlanweb/1Panel"
PROXY="${GITHUB_PROXY:-}"
[ -n "$PROXY" ] && [ "${PROXY%/}" = "$PROXY" ] && PROXY="$PROXY/"

red() { printf '\033[31m%s\033[0m\n' "$*" >&2; }
info() { printf '\033[32m>>\033[0m %s\n' "$*"; }

[ "$(id -u)" -eq 0 ] || { red "请使用 root 运行 / please run as root"; exit 1; }
[ "$(uname -s)" = "Linux" ] || { red "仅支持 Linux / Linux only"; exit 1; }
command -v curl >/dev/null || { red "需要 curl / curl is required"; exit 1; }
command -v tar >/dev/null || { red "需要 tar / tar is required"; exit 1; }

case "$(uname -m)" in
x86_64 | amd64) ARCH=amd64 ;;
aarch64 | arm64) ARCH=arm64 ;;
*) red "不支持的架构 / unsupported arch: $(uname -m)"; exit 1 ;;
esac

if command -v 1panel >/dev/null 2>&1 || command -v 1pctl >/dev/null 2>&1; then
	red "检测到已安装 1Panel，请在面板页脚点击“更新”升级到最新版本。"
	red "1Panel is already installed; use the Update button in the panel footer."
	exit 1
fi

VERSION="${PANEL_VERSION:-}"
if [ -z "$VERSION" ]; then
	# Follow the /releases/latest redirect (no API token / rate limit needed).
	VERSION="$(curl -fsSIL -o /dev/null -w '%{url_effective}' "${PROXY}https://github.com/$REPO/releases/latest" | sed -n 's#.*/tag/##p')"
	[ -n "$VERSION" ] || { red "获取最新版本失败，可设置 PANEL_VERSION=v2.YYYYMMDD.N 或 GITHUB_PROXY 后重试"; exit 1; }
fi
PKG="1panel-$VERSION-linux-$ARCH"
BASE="${PROXY}https://github.com/$REPO/releases/download/$VERSION"
info "安装版本 / version: $VERSION ($ARCH)"

WORK="$(mktemp -d /tmp/1panel-install.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT
cd "$WORK"

info "下载 / downloading $PKG.tar.gz"
curl -fL --retry 5 --retry-delay 3 -C - -o "$PKG.tar.gz" "$BASE/$PKG.tar.gz"
curl -fsSL --retry 5 -o "$PKG.tar.gz.sha256" "$BASE/$PKG.tar.gz.sha256"

info "校验 / verifying sha256"
want="$(awk '{print $1}' "$PKG.tar.gz.sha256")"
if command -v sha256sum >/dev/null; then
	got="$(sha256sum "$PKG.tar.gz" | awk '{print $1}')"
else
	got="$(shasum -a 256 "$PKG.tar.gz" | awk '{print $1}')"
fi
[ -n "$want" ] && [ "$want" = "$got" ] || { red "sha256 校验失败 / checksum mismatch"; exit 1; }

tar -xzf "$PKG.tar.gz"
[ -f "$PKG/install.sh" ] || { red "安装包结构异常 / install.sh missing"; exit 1; }

info "开始安装 / running install.sh"
cd "$PKG"
# install.sh reads answers from the terminal; keep stdin attached to it even
# when this script itself was piped in.
if [ -t 0 ] || [ ! -e /dev/tty ]; then
	bash install.sh "$@"
else
	bash install.sh "$@" </dev/tty
fi
