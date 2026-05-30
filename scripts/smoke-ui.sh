#!/usr/bin/env bash
set -euo pipefail

USER_BASE="${USER_BASE:-http://localhost:8080}"
ADMIN_BASE="${ADMIN_BASE:-http://localhost:8083}"
GW_BASE="${GW_BASE:-http://localhost:8081}"

echo "[smoke-ui] checking health endpoints..."
curl -fsS "${USER_BASE}/healthz" >/dev/null
curl -fsS "${ADMIN_BASE}/healthz" >/dev/null
curl -fsS "${GW_BASE}/healthz" >/dev/null

echo "[smoke-ui] checking key pages..."
curl -fsS "${USER_BASE}/app.html" | rg -q "chatTestSection"
curl -fsS "${USER_BASE}/app.html" | rg -q "chatTestHistorySelect"
curl -fsS "${ADMIN_BASE}/admin.html" | rg -q "adminTabs"
curl -fsS "${ADMIN_BASE}/admin.html" | rg -q "monitorFocusHighError"

echo "[smoke-ui] checking static assets..."
curl -fsS "${USER_BASE}/js/app.js" | rg -q "buildChatTestRequestContext"
curl -fsS "${USER_BASE}/js/app.js" | rg -q "renderChatTestHistoryOptions"
curl -fsS "${ADMIN_BASE}/js/admin.js" | rg -q "monitorFocusFilter"

echo "[smoke-ui] PASS"
