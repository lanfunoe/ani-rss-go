#!/bin/sh
# Docker 入口：以 PUID:PGID 与 UMASK 运行 run.sh

chown -R "${PUID}:${PGID}" /usr/app

umask "${UMASK}"

exec su-exec "${PUID}:${PGID}" /usr/app/run.sh
