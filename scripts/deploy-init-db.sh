#!/usr/bin/env bash
# 生产/预发环境数据库初始化：建库 → 顺序执行 migrations → 导入 baseline seed
#
# 用法:
#   DB_HOST=127.0.0.1 DB_PORT=3306 DB_USER=root DB_PASSWORD=secret DB_NAME=traffic_ai \
#     ./scripts/deploy-init-db.sh
#
# 依赖: mysql 客户端；可选 golang-migrate（优先使用，会写入 schema_migrations 版本表）
#
# 注意:
#   - 全新库: 执行全部 migrations + seed
#   - 已有库且用过 golang-migrate: 仅 migrate up 增量
#   - 已有库但未用过 golang-migrate: 脚本会按序尝试执行 .up.sql（部分 migration 可能已应用，需人工确认）
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

DB_HOST="${DB_HOST:-127.0.0.1}"
DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-root}"
DB_PASSWORD="${DB_PASSWORD:-}"
DB_NAME="${DB_NAME:-traffic_ai}"

MYSQL=(mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER")
if [ -n "$DB_PASSWORD" ]; then
  export MYSQL_PWD="$DB_PASSWORD"
fi

log() { echo "[deploy-init-db] $*"; }
fail() { echo "[deploy-init-db] ERROR: $*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "缺少命令: $1"
}

require_cmd mysql

log "检查 MySQL 连通性 (${DB_USER}@${DB_HOST}:${DB_PORT}) …"
"${MYSQL[@]}" -e "SELECT 1" >/dev/null || fail "无法连接 MySQL"

log "创建数据库 ${DB_NAME}（若不存在）…"
"${MYSQL[@]}" -e "CREATE DATABASE IF NOT EXISTS \`${DB_NAME}\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

migrate_with_golang_migrate() {
  command -v golang-migrate >/dev/null 2>&1 || return 1
  local dsn
  if [ -n "$DB_PASSWORD" ]; then
    # golang-migrate 要求 URL 编码密码中的特殊字符；简单场景用原始密码，复杂密码请手动 migrate
    dsn="mysql://${DB_USER}:${DB_PASSWORD}@tcp(${DB_HOST}:${DB_PORT})/${DB_NAME}?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true"
  else
    dsn="mysql://${DB_USER}@tcp(${DB_HOST}:${DB_PORT})/${DB_NAME}?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true"
  fi
  log "使用 golang-migrate 执行 migrations …"
  golang-migrate -path "$PROJECT_DIR/migrations" -database "$dsn" up
  return 0
}

migrate_with_mysql_loop() {
  log "未检测到 golang-migrate，按文件名顺序执行 migrations/*.up.sql …"
  local applied=0
  for f in "$PROJECT_DIR"/migrations/[0-9]*_*.up.sql; do
    [ -f "$f" ] || continue
    log "  → $(basename "$f")"
    "${MYSQL[@]}" "$DB_NAME" < "$f"
    applied=$((applied + 1))
  done
  [ "$applied" -gt 0 ] || fail "未找到任何 migration 文件"
}

if migrate_with_golang_migrate; then
  :
else
  migrate_with_mysql_loop
fi

if [ -d "$PROJECT_DIR/deploy/seed" ]; then
  shopt -s nullglob
  seed_files=("$PROJECT_DIR"/deploy/seed/*.sql)
  shopt -u nullglob
  if [ "${#seed_files[@]}" -gt 0 ]; then
    log "导入 baseline seed …"
    for seed in "${seed_files[@]}"; do
      log "  → $(basename "$seed")"
      "${MYSQL[@]}" "$DB_NAME" < "$seed"
    done
  fi
fi

TABLE_COUNT=$("${MYSQL[@]}" -N -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='${DB_NAME}';")
USER_TABLE=$("${MYSQL[@]}" -N "$DB_NAME" -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='${DB_NAME}' AND table_name='users';")
MIGRATION_VER=$("${MYSQL[@]}" -N "$DB_NAME" -e "SELECT COALESCE(MAX(version), 'n/a') FROM schema_migrations;" 2>/dev/null || echo "n/a (no schema_migrations)")

log "完成。库 ${DB_NAME}: ${TABLE_COUNT} 张表，users 表存在=${USER_TABLE}，migration 版本=${MIGRATION_VER}"
log "下一步: 通过 /auth/register 创建首个用户，再提权 super_admin（见 docs/deploy.md）"
