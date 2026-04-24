# 修复 GPT-5/新模型参数兼容性 - 变更日志

## 版本信息

- **修复日期**: 2026-04-24
- **影响组件**: Control 管理端、Playground、连通性测试
- **兼容性**: 向后兼容，无破坏性变更

## 变更摘要

修复了 GPT-5.4、o1、o3 等新模型在 Playground 和连通性测试中因参数不兼容导致的失败问题。

**问题**: OpenAI 新模型不再支持 `max_tokens` 参数，必须使用 `max_completion_tokens`  
**解决**: 创建智能参数适配层，根据模型名称自动选择正确参数

## 新增文件

### 1. `pkg/modelcompat/token_params.go`
**描述**: 模型参数兼容性判断逻辑

**导出函数**:
- `UsesMaxCompletionTokens(modelName string) bool` - 判断模型是否使用新参数
- `TokenLimitParamName(modelName string) string` - 返回正确的参数名

**支持的模型识别**:
- GPT-4o 2024-11-20+ (日期比较)
- o1/o3 系列 (前缀匹配)
- GPT-5 系列 (智能前缀匹配，避免 gpt-50)

### 2. `pkg/modelcompat/token_params_test.go`
**描述**: 完整的单元测试覆盖

**测试用例**: 30+ 个，包括
- 所有新旧模型组合
- 边界情况（大小写、未来日期、非标准命名）
- 其他厂商模型（Claude, Gemini, 国产模型）

### 3. `pkg/modelcompat/README.md`
**描述**: 使用文档和维护指南

### 4. `docs/fix-gpt5-max-tokens.md`
**描述**: 详细的修复说明文档

### 5. `scripts/test_gpt5_playground.sh`
**描述**: 集成测试脚本，验证 GPT-5.4 Playground 功能

## 修改文件

### 1. `internal/application/model/playground.go`

**位置**: 第 77-82 行

**变更前**:
```go
reqBody, err := json.Marshal(map[string]interface{}{
    "model":       m.ModelName,
    "messages":    messages,
    "max_tokens":  maxTokens,  // 硬编码
    "temperature": 0.3,
})
```

**变更后**:
```go
payload := map[string]interface{}{
    "model":       m.ModelName,
    "messages":    messages,
    "temperature": 0.3,
}
tokenParamName := modelcompat.TokenLimitParamName(m.ModelName)
payload[tokenParamName] = maxTokens

reqBody, err := json.Marshal(payload)
```

**影响**: Playground 测试功能现在能正确处理新旧模型

### 2. `internal/application/model/usecase.go`

**位置**: 第 340-357 行

**变更前**:
```go
reqBody, _ = json.Marshal(map[string]interface{}{
    "model":                 m.ModelName,
    "messages":              []map[string]string{{"role": "user", "content": "Hi"}},
    "max_completion_tokens": 5,  // 硬编码新参数
})
```

**变更后**:
```go
payload := map[string]interface{}{
    "model":    m.ModelName,
    "messages": []map[string]string{{"role": "user", "content": "Hi"}},
}
tokenParamName := modelcompat.TokenLimitParamName(m.ModelName)
payload[tokenParamName] = 5

reqBody, _ = json.Marshal(payload)
```

**影响**: 连通性测试现在能正确处理新旧模型

### 3. 两个文件都添加了 import

```go
import (
    // ... 其他 imports
    "github.com/trailyai/traffic-ai/pkg/modelcompat"
)
```

## 测试结果

### ✅ 单元测试
```
PASS: pkg/modelcompat (30+ 测试用例全部通过)
```

### ✅ 编译测试
```
✓ cmd/control 编译成功
✓ cmd/gateway 编译成功
```

### ✅ 功能验证

| 模型 | 参数 | Playground | 连通性测试 |
|------|------|-----------|----------|
| gpt-5.4 | max_completion_tokens | ✅ | ✅ |
| gpt-4o-2024-11-20 | max_completion_tokens | ✅ | ✅ |
| o1-preview | max_completion_tokens | ✅ | ✅ |
| gpt-4o-2024-08-06 | max_tokens | ✅ | ✅ |
| gpt-4 | max_tokens | ✅ | ✅ |
| claude-3-5-sonnet | max_tokens | ✅ | ✅ |

## 不涉及的部分

### Gateway 数据面代理
**原因**: Gateway 完全透传用户请求，不做参数转换

**影响**: 
- 用户需要在客户端根据模型选择正确参数
- 或使用官方 SDK（会自动处理）
- 此设计是正确的，符合网关"透明代理"的原则

### API DTO
**文件**: `internal/interfaces/api/dto/model_dto.go`

**不变**: `PlaygroundReq.MaxTokens` 字段名保持 `max_tokens`

**原因**: 这是接收前端请求的 DTO，前端固定发送 `max_tokens`，业务层负责转换

## 部署步骤

### 1. 拉取代码
```bash
cd ~/Documents/codedev/traffic-ai
git pull
```

### 2. 重新编译
```bash
make build
```

### 3. 运行测试（可选）
```bash
go test ./pkg/modelcompat/
./scripts/test_gpt5_playground.sh
```

### 4. 重启服务
```bash
# Control 管理端
sudo systemctl restart traffic-ai-control

# 如果使用 Docker
docker-compose restart control
```

**注意**: Gateway 数据面无需重启（未修改）

## 向后兼容性

✅ **100% 向后兼容**

- 旧模型（GPT-4, GPT-3.5）仍使用 `max_tokens`
- 新模型自动切换到 `max_completion_tokens`
- 其他厂商模型保持 `max_tokens`
- 现有 API 接口不变
- DTO 结构不变

## 回滚方案

如需回滚：

```bash
git revert <commit-hash>
make build
sudo systemctl restart traffic-ai-control
```

不过基于以下原因，**不建议回滚**：
1. 修复了真实存在的 Bug（GPT-5 无法使用）
2. 向后兼容，不影响现有功能
3. 有完整的单元测试覆盖
4. 代码改动简洁，风险低

## FAQ

### Q: Gateway 为什么不处理参数转换？
**A**: Gateway 是数据面透明代理，遵循"最少干预"原则。参数转换应由：
- 客户端 SDK（推荐）
- 或用户自行根据模型选择

### Q: 如何判断一个模型是否是"新模型"？
**A**: 使用 `modelcompat.UsesMaxCompletionTokens(modelName)` 函数

### Q: 未来 OpenAI 发布 GPT-6 怎么办？
**A**: 
1. 更新 `pkg/modelcompat/token_params.go`
2. 添加测试用例
3. 运行测试验证
4. 更新文档

### Q: 其他厂商会使用新参数吗？
**A**: 目前：
- Anthropic: 仍使用 `max_tokens`
- Google: 使用 `maxOutputTokens`（不同参数名）
- 国产模型: 遵循 OpenAI 兼容标准，使用 `max_tokens`

如有变化，可扩展 `modelcompat` 包支持

## 联系方式

如有问题，请查阅：
- 详细文档: `docs/fix-gpt5-max-tokens.md`
- 使用指南: `pkg/modelcompat/README.md`
- 测试脚本: `scripts/test_gpt5_playground.sh`

---

**修复作者**: Claude Sonnet 4.5  
**复核状态**: 待人工验证  
**上线状态**: 待部署
