#!/usr/bin/env bash
# 构建 ani-rss-go 发布产物：构建并内嵌 ani-rss-ui，注入版本信息，交叉编译到 dist/
#
# 用法：./build.sh [GOOS/GOARCH[/GOARM] ...]，不传目标时构建全部发布平台
# 环境变量：
#   VERSION  版本号，默认取 git describe 结果（去掉前缀 v），无 tag 时为 dev
#   COMMIT   提交 ID，默认读取 git
#   BRANCH   分支名，默认读取 git
#   SKIP_UI  为 1 时跳过前端构建，直接使用已有的 ani-rss-ui/dist
set -euo pipefail

cd "$(dirname "$0")"
ROOT=$(cd .. && pwd)
UI_DIR="$ROOT/ani-rss-ui"
EMBED_DIR=internal/web/dist
OUT_DIR=dist

VERSION=${VERSION:-$(git -C "$ROOT" describe --tags --match 'v[0-9]*' --dirty 2>/dev/null || true)}
VERSION=${VERSION#v}
VERSION=${VERSION:-dev}
COMMIT=${COMMIT:-$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || true)}
BRANCH=${BRANCH:-$(git -C "$ROOT" rev-parse --abbrev-ref HEAD 2>/dev/null || true)}

if [ "${SKIP_UI:-0}" != 1 ]; then
  (cd "$UI_DIR" && pnpm install --no-frozen-lockfile && pnpm build)
fi
if [ ! -f "$UI_DIR/dist/index.html" ]; then
  echo "缺少前端构建产物 $UI_DIR/dist/index.html" >&2
  exit 1
fi
find "$EMBED_DIR" -mindepth 1 -maxdepth 1 ! -name .gitkeep -exec rm -rf {} +
cp -R "$UI_DIR/dist/." "$EMBED_DIR/"

PKG=ani-rss-go/internal/buildinfo
LDFLAGS="-s -w -X $PKG.Version=$VERSION -X $PKG.Commit=$COMMIT -X $PKG.Branch=$BRANCH"

TARGETS=("$@")
if [ ${#TARGETS[@]} -eq 0 ]; then
  TARGETS=(
    linux/amd64 linux/arm64 linux/arm/7
    windows/amd64 windows/arm64
    darwin/amd64 darwin/arm64
  )
fi

mkdir -p "$OUT_DIR"
for target in "${TARGETS[@]}"; do
  IFS=/ read -r goos goarch goarm <<<"$target"
  name="ani-rss-go-$goos-$goarch${goarm:+v$goarm}"
  if [ "$goos" = windows ]; then
    name+=.exe
  fi
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch GOARM=$goarm \
    go build -trimpath -ldflags "$LDFLAGS" -o "$OUT_DIR/$name" .
  echo "已构建 $OUT_DIR/$name ($VERSION)"
done
