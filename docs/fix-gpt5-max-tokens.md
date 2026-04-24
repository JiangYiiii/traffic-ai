# 修复 GPT-5/新模型 max_tokens 参数兼容性问题

## 问题描述

### 现象

GPT-5.4 模型在 Playground 测试时报错：

```json
{
  "error": {
    "message": "Unsupported parameter: 'max_tokens' is not supported with this model. Use 'max_completion_tokens' instead.",
    "type": "invalid_request_error",
    "param": "max_tokens",
    "code": "unsupported_parameter"
  }
}
```

### 根本原因

OpenAI 从 2024-12-01 开始，在新一代模型中弃用 `max_tokens` 参数，改用 `max_completion_tokens`：

- **新模型**：GPT-4o 2024-11-20+、o1 系列、o3 系列、GPT-5 系列
- **旧模型**：GPT-4o 2024-11-01 及之前、GPT-4、GPT-3.5
- **其他厂商**：Anthropic、Google、国产模型均仍使用 `max_tokens`

Traffic AI 之前硬编码使用 `max_tokens`（Playground）或 `max_completion_tokens`（连通性测试），导致部分模型无法正常工作。

## 解决方案

### 1. 创建模型兼容性包

**位置**: `pkg/modelcompat/`

**功能**: 根据模型名称智能判断应使用哪个参数

**核心 API**:

```go
// 判断模型是否使用新参数
modelcompat.UsesMaxCompletionTokens("gpt-5.4")  // → true
modelcompat.UsesMaxCompletionTokens("gpt-4")    // → false

// 获取正确的参数名
modelcompat.TokenLimitParamName("gpt-5.4")  // → "max_completion_tokens"
modelcompat.TokenLimitParamName("gpt-4")    // → "max_tokens"
```

**识别规则**:

1. **GPT-4o 日期判断**: 使用 ISO 8601 日期字典序比较 `>= "2024-11-20"`
2. **o1/o3 系列**: 前缀匹配 `o1-`, `o1`, `o3-`, `o3`
3. **GPT-5 系列**: 前缀 `gpt-5` 且后面不是数字（避免 `gpt-50`）

### 2. 修改 Playground

**文件**: `internal/application/model/playground.go`

**修改前**:
```go
reqBody, err := json.Marshal(map[string]interface{}{
    "model":       m.ModelName,
    "messages":    messages,
    "max_tokens":  maxTokens,  // ❌ 硬编码
    "temperature": 0.3,
})
```

**修改后**:
```go
payload := map[string]interface{}{
    "model":       m.ModelName,
    "messages":    messages,
    "temperature": 0.3,
}
tokenParamName := modelcompat.TokenLimitParamName(m.ModelName)  // ✅ 动态选择
payload[tokenParamName] = maxTokens

reqBody, err := json.Marshal(payload)
```

### 3. 修改连通性测试

**文件**: `internal/application/model/usecase.go`

**修改前**:
```go
reqBody, _ = json.Marshal(map[string]interface{}{
    "model":                 m.ModelName,
    "messages":              []map[string]string{{"role": "user", "content": "Hi"}},
    "max_completion_tokens": 5,  // ❌ 硬编码新参数，导致旧模型报错
})
```

**修改后**:
```go
payload := map[string]interface{}{
    "model":    m.ModelName,
    "messages": []map[string]string{{"role": "user", "content": "Hi"}},
}
tokenParamName := modelcompat.TokenLimitParamName(m.ModelName)  // ✅ 动态选择
payload[tokenParamName] = 5

reqBody, _ = json.Marshal(payload)
```

## 影响范围

### ✅ 已修复

1. **Playground 测试**: 管理端模型测试功能
2. **连通性测试**: 模型账号健康检查

### ⚠️ 未涉及（无需修改）

3. **Gateway 数据面代理**: 完全透传用户请求，不做参数转换
   - 用户应在客户端根据模型选择正确参数
   - 或使用 SDK（SDK 会自动处理）

## 测试验证

### 单元测试

```bash
cd ~/Documents/codedev/traffic-ai
go test -v ./pkg/modelcompat/
```

**测试覆盖**:
- 30+ 个测试用例
- 所有新旧模型组合
- 边界情况（大小写、未来模型、非官方命名）

### 集成测试

```bash
cd ~/Documents/codedev/traffic-ai
./scripts/test_gpt5_playground.sh
```

**测试场景**:
- GPT-5.4 Playground 请求
- 验证不再报错 "Unsupported parameter"
- 确认能正常获得响应

### 手动测试

1. **测试 GPT-5.4 (新参数)**:
```bash
curl 'http://localhost:3002/admin/models/3/playground' \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  --data-raw '{"messages":[{"role":"user","content":"你好"}],"max_tokens":256}'
```

预期：成功返回，不报错

2. **测试 GPT-4 (旧参数)**:
```bash
curl 'http://localhost:3002/admin/models/1/playground' \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  --data-raw '{"messages":[{"role":"user","content":"Hello"}],"max_tokens":256}'
```

预期：成功返回，不报错

## 部署说明

### 构建

```bash
cd ~/Documents/codedev/traffic-ai
make build
```

### 重启服务

```bash
# 如果使用 systemd
sudo systemctl restart traffic-ai-control
sudo systemctl restart traffic-ai-gateway

# 或使用 Docker
docker-compose restart control gateway

# 或直接运行
./control -config configs/config.yaml
./gateway -config configs/config.yaml
```

### 回滚方案

如果出现问题，可以回滚到修改前的版本：

```bash
git revert <commit-hash>
make build
# 重启服务
```

不过本次修改**向后兼容**，不会影响现有功能：
- 旧模型仍使用 `max_tokens` ✅
- 新模型改用 `max_completion_tokens` ✅
- 其他厂商模型使用 `max_tokens` ✅

## 未来维护

### 新模型支持

当 OpenAI 发布新模型系列（如 GPT-6）时：

1. 更新 `pkg/modelcompat/token_params.go`
2. 添加对应测试用例到 `token_params_test.go`
3. 更新 `pkg/modelcompat/README.md` 文档
4. 运行测试验证

### 其他厂商

如果 Anthropic、Google 或国产模型未来也采用新参数：

1. 在 `UsesMaxCompletionTokens()` 函数中添加对应逻辑
2. 添加测试用例验证
3. 更新文档

## 参考资料

- [OpenAI API 文档 - 2024-12-01](https://platform.openai.com/docs/api-reference/chat/create)
- [OpenAI 模型版本说明](https://platform.openai.com/docs/models)
- Traffic AI 项目文档: `docs/project-doc.md`

## 相关 Issue

- 原始问题：GPT-5.4 Playground 报错 "Unsupported parameter"
- 修复时间：2026-04-24
- 影响版本：所有使用 GPT-4o 2024-11-20+、o1、o3、GPT-5 的部署
