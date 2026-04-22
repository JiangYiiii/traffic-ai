#!/usr/bin/env bash
# save-snapshot.sh — 校验采集数据完整性 + 保存快照 + 更新 index.json
#
# 用法:
#   save-snapshot.sh --business <id> --scenario <name> --phase before|after \
#     --keywords '<json>' --data-file <path> [--append <path>]
#
# 退出码:
#   0 = 完整，已保存
#   1 = 不完整，返回缺失表清单 JSON
#   2 = 参数/配置错误

set -euo pipefail

# 从脚本所在目录向上查找含 .observatory 的项目根（兼容 tools/ 与 .cursor/skills/.../tools/）
find_project_root() {
  local dir="$1"
  while [[ "$dir" != "/" ]]; do
    if [[ -d "$dir/.observatory" ]]; then
      echo "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  return 1
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if ! PROJECT_ROOT="$(find_project_root "$SCRIPT_DIR")"; then
  echo "找不到项目根（向上查找含 .observatory 的目录）" >&2
  exit 2
fi
CONFIG_DIR="${PROJECT_ROOT}/.observatory/snapshots/config"
BUSINESSES_JSON="${CONFIG_DIR}/businesses-tables.json"
PROVIDER_JSON="${CONFIG_DIR}/query-provider.json"

BUSINESS=""
SCENARIO=""
PHASE=""
KEYWORDS=""
DATA_FILE=""
APPEND_FILE=""

usage() {
  echo "用法: $0 --business <id> --scenario <name> --phase before|after --keywords '<json>' --data-file <path> [--append <path>]" >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --business)  BUSINESS="$2"; shift 2 ;;
    --scenario)  SCENARIO="$2"; shift 2 ;;
    --phase)     PHASE="$2"; shift 2 ;;
    --keywords)  KEYWORDS="$2"; shift 2 ;;
    --data-file) DATA_FILE="$2"; shift 2 ;;
    --append)    APPEND_FILE="$2"; shift 2 ;;
    *) echo "未知参数: $1" >&2; usage ;;
  esac
done

[[ -z "$BUSINESS" || -z "$SCENARIO" || -z "$PHASE" || -z "$KEYWORDS" || -z "$DATA_FILE" ]] && usage
[[ "$PHASE" != "before" && "$PHASE" != "after" ]] && { echo "phase 必须为 before 或 after" >&2; exit 2; }
[[ ! -f "$DATA_FILE" ]] && { echo "数据文件不存在: $DATA_FILE" >&2; exit 2; }
[[ ! -f "$BUSINESSES_JSON" ]] && { echo "配置文件不存在: $BUSINESSES_JSON" >&2; exit 2; }

resolve_data_dir() {
  local dir
  if [[ -f "$PROVIDER_JSON" ]]; then
    dir=$(jq -r '.storage.data_dir // ".observatory/snapshots/data"' "$PROVIDER_JSON")
  else
    dir=".observatory/snapshots/data"
  fi
  if [[ "$dir" = /* ]]; then
    echo "$dir"
  else
    echo "${PROJECT_ROOT}/${dir}"
  fi
}

DATA_DIR="$(resolve_data_dir)"

# ── 1. 如有 --append，合并到 data-file ──
if [[ -n "$APPEND_FILE" && -f "$APPEND_FILE" ]]; then
  MERGED=$(mktemp)
  jq -s '
    def deep_merge:
      reduce .[] as $item ({}; . as $base |
        ($item | to_entries[]) as $e |
        $base + {($e.key): (
          if ($base[$e.key] | type) == "object" and ($e.value | type) == "object"
          then [$base[$e.key], $e.value] | deep_merge
          else $e.value
          end
        )}
      );
    deep_merge
  ' "$DATA_FILE" "$APPEND_FILE" > "$MERGED"
  cp "$MERGED" "$DATA_FILE"
  rm -f "$MERGED"
fi

# ── 2. 读取该业务期望的表列表 ──
BIZ_EXISTS=$(jq -r --arg b "$BUSINESS" '.businesses[$b] // empty' "$BUSINESSES_JSON")
if [[ -z "$BIZ_EXISTS" ]]; then
  echo "{\"error\": \"业务 '$BUSINESS' 不存在于 businesses-tables.json\"}" >&2
  exit 2
fi

EXPECTED_TABLES=$(jq -r --arg b "$BUSINESS" '
  [.businesses[$b].tables[] | "\(.database).\(.table)"] | sort | .[]
' "$BUSINESSES_JSON")

EXPECTED_COUNT=$(echo "$EXPECTED_TABLES" | grep -c . || true)

# ── 3. 读取 data-file 中已采集的表 ──
COLLECTED_TABLES=$(jq -r '
  [to_entries[] | .key as $db | .value | to_entries[] | "\($db).\(.key)"] | sort | .[]
' "$DATA_FILE")

# ── 4. 对比：期望 - 已采集 = 缺失 ──
MISSING=$(comm -23 <(echo "$EXPECTED_TABLES" | sort) <(echo "$COLLECTED_TABLES" | sort))

COLLECTED_COUNT=$(echo "$COLLECTED_TABLES" | grep -c . || true)

if [[ -n "$MISSING" ]]; then
  MISSING_COUNT=$(echo "$MISSING" | grep -c . || true)
  MISSING_JSON=$(echo "$MISSING" | while IFS= read -r line; do
    db="${line%%.*}"
    tbl="${line#*.}"
    sql=$(jq -r --arg b "$BUSINESS" --arg t "$tbl" '
      .businesses[$b].tables[] | select(.table == $t) | .query_sql // ""
    ' "$BUSINESSES_JSON")
    jq -n --arg db "$db" --arg tbl "$tbl" --arg sql "$sql" \
      '{database: $db, table: $tbl, query_sql: $sql}'
  done | jq -s '.')

  jq -n \
    --arg status "incomplete" \
    --arg phase "$PHASE" \
    --argjson collected "$COLLECTED_COUNT" \
    --argjson expected "$EXPECTED_COUNT" \
    --argjson missing_tables "$MISSING_JSON" \
    --argjson missing_count "$MISSING_COUNT" \
    '{
      status: $status,
      phase: $phase,
      collected_count: $collected,
      expected_count: $expected,
      missing_tables: $missing_tables,
      message: "缺失 \($missing_count) 张表，请继续采集以下表的数据，采集完成后再次调用 save-snapshot.sh（使用 --append 追加模式）"
    }'
  exit 1
fi

# ── 5. 完整 → 封装保存 ──

BIZ_NAME=$(jq -r --arg b "$BUSINESS" '.businesses[$b].name // $b' "$BUSINESSES_JSON")

TIMESTAMP=$(date +"%Y%m%d_%H%M%S")
ISO_NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
TS_MS=$(($(date +%s) * 1000))

SCENARIO_SLUG=$(echo "$SCENARIO" | sed 's|[/\\:*?"<>|]|_|g; s/__*/_/g; s/^_//; s/_$//')
[[ -z "$SCENARIO_SLUG" ]] && SCENARIO_SLUG="unnamed"

CASE_DIR="${DATA_DIR}/${BUSINESS}/${SCENARIO_SLUG}/${TIMESTAMP}"
mkdir -p "$CASE_DIR"

SNAPSHOT_JSON=$(jq -n \
  --arg time "$ISO_NOW" \
  --argjson ts "$TS_MS" \
  --argjson kw "$KEYWORDS" \
  --argjson data "$(cat "$DATA_FILE")" \
  '{snapshot_time: $time, snapshot_timestamp_ms: $ts, keywords: $kw, data: $data}')

echo "$SNAPSHOT_JSON" > "${CASE_DIR}/${PHASE}.json"

# ── 6. 更新 index.json ──

INDEX_FILE="${DATA_DIR}/index.json"

if [[ ! -f "$INDEX_FILE" ]]; then
  echo '{"version":"1.0","cases":[]}' > "$INDEX_FILE"
fi

CASE_ID="${BUSINESS}__${SCENARIO_SLUG}__${TIMESTAMP}"
REL_PATH="${BUSINESS}/${SCENARIO_SLUG}/${TIMESTAMP}"

HAS_BEFORE=false
HAS_AFTER=false
[[ "$PHASE" == "before" ]] && HAS_BEFORE=true
[[ "$PHASE" == "after" ]] && HAS_AFTER=true

EXISTING_CASE=$(jq -r --arg bid "$BUSINESS" --arg sc "$SCENARIO_SLUG" '
  .cases[] | select(.business_id == $bid and .scenario_slug == $sc) | .data_path // empty
' "$INDEX_FILE" | head -1)

if [[ -n "$EXISTING_CASE" && "$PHASE" == "after" ]]; then
  EXISTING_DIR="${DATA_DIR}/${EXISTING_CASE}"
  if [[ -d "$EXISTING_DIR" && -f "${EXISTING_DIR}/before.json" && ! -f "${EXISTING_DIR}/after.json" ]]; then
    cp "${CASE_DIR}/after.json" "${EXISTING_DIR}/after.json"
    rm -rf "$CASE_DIR"
    CASE_DIR="$EXISTING_DIR"
    REL_PATH="$EXISTING_CASE"
    CASE_ID=$(basename "$(dirname "$EXISTING_CASE")")__$(basename "$EXISTING_CASE")
    CASE_ID="${BUSINESS}__${SCENARIO_SLUG}__$(basename "$EXISTING_CASE")"
    HAS_BEFORE=true
    HAS_AFTER=true

    jq --arg dp "$EXISTING_CASE" '
      .cases = [.cases[] | if .data_path == $dp then .has_after = true else . end]
    ' "$INDEX_FILE" > "${INDEX_FILE}.tmp" && mv "${INDEX_FILE}.tmp" "$INDEX_FILE"

    SAVED_PATH="${EXISTING_DIR}/after.json"
    jq -n \
      --arg status "complete" \
      --arg phase "$PHASE" \
      --argjson collected "$COLLECTED_COUNT" \
      --argjson expected "$EXPECTED_COUNT" \
      --arg saved_path "$SAVED_PATH" \
      '{
        status: $status,
        phase: $phase,
        collected_count: $collected,
        expected_count: $expected,
        saved_path: $saved_path,
        index_updated: true,
        message: "结果数据采集完成，共 \($collected) 张表，已追加到已有 case 并更新索引"
      }'
    exit 0
  fi
fi

NEW_ENTRY=$(jq -n \
  --arg id "$CASE_ID" \
  --arg bid "$BUSINESS" \
  --arg bname "$BIZ_NAME" \
  --arg sc "$SCENARIO" \
  --arg sc_slug "$SCENARIO_SLUG" \
  --argjson kw "$KEYWORDS" \
  --arg created "$ISO_NOW" \
  --argjson hb "$HAS_BEFORE" \
  --argjson ha "$HAS_AFTER" \
  --arg dp "$REL_PATH" \
  '{
    id: $id,
    business_id: $bid,
    business_name: $bname,
    scenario: $sc,
    scenario_slug: $sc_slug,
    keywords: $kw,
    created_at: $created,
    has_before: $hb,
    has_after: $ha,
    data_path: $dp
  }')

jq --argjson entry "$NEW_ENTRY" '.cases += [$entry]' "$INDEX_FILE" > "${INDEX_FILE}.tmp" \
  && mv "${INDEX_FILE}.tmp" "$INDEX_FILE"

SAVED_PATH="${CASE_DIR}/${PHASE}.json"
PHASE_LABEL=$( [[ "$PHASE" == "before" ]] && echo "初始" || echo "结果" )

jq -n \
  --arg status "complete" \
  --arg phase "$PHASE" \
  --argjson collected "$COLLECTED_COUNT" \
  --argjson expected "$EXPECTED_COUNT" \
  --arg saved_path "$SAVED_PATH" \
  '{
    status: $status,
    phase: $phase,
    collected_count: $collected,
    expected_count: $expected,
    saved_path: $saved_path,
    index_updated: true,
    message: "\($phase)数据采集完成，共 \($collected) 张表，已保存并更新索引"
  }'
exit 0
