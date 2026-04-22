#!/usr/bin/env bash
# 已弃用：请使用 OpenClaw 内置 Gateway cron 安装:
#   ./scripts/openclaw-cron-install-traffic-sync.sh
REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
echo "此 launchd 安装方式已弃用。请改执行:" >&2
echo "  $REPO_ROOT/scripts/openclaw-cron-install-traffic-sync.sh" >&2
echo "由 OpenClaw 在 ~/.openclaw/cron/jobs.json 中持久化定时任务，无需 launchd。" >&2
exit 1
