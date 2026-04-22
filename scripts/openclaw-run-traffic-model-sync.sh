#!/bin/sh
# 供 OpenClaw 内置 cron（仅 exec 工具）在本机子进程中调用；可在此设置与同步相关的环境变量。
# 与 scripts/openclaw-cron-install-traffic-sync.sh 成对使用。
# shellcheck disable=SC1091
SYNC_DIR="$(cd "$(dirname "$0")" && pwd)"
: "${OPENCLAW_SYNC_RESTART:=0}"
: "${OPENCLAW_CONFIG:=$HOME/.openclaw/openclaw.json}"
export OPENCLAW_SYNC_RESTART
export OPENCLAW_CONFIG
exec /usr/bin/python3 "$SYNC_DIR/openclaw-sync-traffic-models.py" "$@"
