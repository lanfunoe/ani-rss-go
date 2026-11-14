#!/bin/sh
# 循环运行同目录下的 ani-rss：退出码为 0 时重启（/api/stop?status=0），否则结束（status=1）
# 用于 Docker 镜像与 macOS 应用

export LANG=C.UTF-8
export LC_ALL=C.UTF-8

BIN="$(cd "$(dirname "$0")" && pwd)/ani-rss"
PID=""

stop() {
  if [ -n "$PID" ]; then
    kill -TERM "$PID" 2>/dev/null
    wait "$PID"
  fi
  exit 0
}

trap stop TERM INT

while :; do
  "$BIN" &
  PID=$!
  wait "$PID"
  if [ $? -ne 0 ]; then
    break
  fi
done

exit 0
