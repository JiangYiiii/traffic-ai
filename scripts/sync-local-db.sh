#!/usr/bin/env bash
# 本地 MySQL 对齐当前仓库 schema：先保证库与基表存在，再执行幂等补丁（000002～000005 等价逻辑）
# 用法: ./scripts/sync-local-db.sh
set -euo pipefail
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
MYSQL=(mysql -u root -h 127.0.0.1)

"${MYSQL[@]}" -e "CREATE DATABASE IF NOT EXISTS traffic_ai DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

HAS_MODELS=$("${MYSQL[@]}" -N traffic_ai -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='traffic_ai' AND table_name='models';" || echo 0)
if [ "${HAS_MODELS}" = "0" ]; then
  echo "[sync-local-db] 导入基线 migrations/000001_init_schema.up.sql …"
  "${MYSQL[@]}" traffic_ai < "$PROJECT_DIR/migrations/000001_init_schema.up.sql"
fi

echo "[sync-local-db] 应用 scripts/ensure_traffic_ai_schema.sql（可重复执行）…"
"${MYSQL[@]}" traffic_ai < "$PROJECT_DIR/scripts/ensure_traffic_ai_schema.sql"
echo "[sync-local-db] 完成。"
