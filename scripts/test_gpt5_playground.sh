#!/bin/bash
# 测试 GPT-5.4 模型的 Playground 功能
# 验证是否正确使用 max_completion_tokens 参数

set -e

TRAFFIC_BASE_URL="${TRAFFIC_BASE_URL:-http://127.0.0.1:18080}"
PLAYGROUND_MODEL_ID="${PLAYGROUND_MODEL_ID:-3}"
if [ -z "${AUTH_TOKEN:-}" ]; then
  echo "请设置环境变量 AUTH_TOKEN（管理端登录后的 Bearer JWT）" >&2
  exit 1
fi

echo "🧪 测试 Traffic AI Playground - GPT-5.4 参数兼容性"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 测试 GPT-5.4 (应使用 max_completion_tokens)
echo "📝 测试 1: GPT-5.4 模型"
echo "预期：使用 max_completion_tokens 参数"
echo ""

RESPONSE=$(curl -s "${TRAFFIC_BASE_URL}/admin/models/${PLAYGROUND_MODEL_ID}/playground" \
  -H "Authorization: Bearer ${AUTH_TOKEN}" \
  -H "Content-Type: application/json" \
  --data-raw '{"messages":[{"role":"user","content":"你好"}],"max_tokens":256}')

echo "响应: $RESPONSE"
echo ""

# 检查是否成功
if echo "$RESPONSE" | jq -e '.data.success == true' > /dev/null 2>&1; then
  echo "✅ 测试通过：GPT-5.4 请求成功"
  ASSISTANT=$(echo "$RESPONSE" | jq -r '.data.assistant')
  echo "   助手回复: $ASSISTANT"
else
  ERROR=$(echo "$RESPONSE" | jq -r '.data.error // .message // "Unknown error"')
  if echo "$ERROR" | grep -q "Unsupported parameter"; then
    echo "❌ 测试失败：仍在使用旧参数 max_tokens"
    echo "   错误信息: $ERROR"
    exit 1
  else
    echo "⚠️  请求失败（可能是其他原因）: $ERROR"
  fi
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "测试完成"
