#!/usr/bin/env bash
# 本地验证 CNB 同款 Docker 构建（不推送 CCR）
# 用法: ./scripts/cnb-build-local.sh [control|gateway|all]
set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="${1:-all}"

build_one() {
  local t="$1"
  echo "[cnb-build-local] building ${t} …"
  docker build -f "${PROJECT_DIR}/Dockerfile" \
    --build-arg "BUILD_TARGET=${t}" \
    -t "traffic-ai-${t}:local" \
    "${PROJECT_DIR}"
  echo "[cnb-build-local] done: traffic-ai-${t}:local"
}

case "$TARGET" in
  control|gateway) build_one "$TARGET" ;;
  all)
    build_one control
    build_one gateway
    ;;
  *)
    echo "用法: $0 [control|gateway|all]" >&2
    exit 1
    ;;
esac
