# Model Compatibility Package

## 背景

OpenAI 从 2024 年 12 月开始，在新一代模型中**弃用** `max_tokens` 参数，改用 `max_completion_tokens`。

### 参数变更时间线

| 日期 | 事件 |
|------|------|
| 2024-11-20 | GPT-4o 新快照版本开始使用新参数 |
| 2024-12-01 | OpenAI API 文档正式说明参数变更 |

### 影响的模型

#### ✅ 使用 `max_completion_tokens` 的模型

1. **GPT-4o 2024-11-20+**
   - `gpt-4o-2024-11-20`
   - `gpt-4o-2024-12-01`
   - `gpt-4o-2025-01-15`（所有未来快照）

2. **o1 系列**
   - `o1-preview`
   - `o1-mini`
   - `o1`

3. **o3 系列**
   - `o3-mini`
   - `o3`

4. **GPT-5 系列**
   - `gpt-5`
   - `gpt-5.4`
   - `gpt-5-turbo`
   - 所有 `gpt-5` 开头的模型（但不包括 `gpt-50`, `gpt-500` 等）

#### ❌ 仍使用 `max_tokens` 的模型

1. **旧版 GPT-4o**
   - `gpt-4o-2024-08-06`
   - `gpt-4o-2024-11-01`（11-20 之前）

2. **GPT-4 系列**
   - `gpt-4`
   - `gpt-4-turbo`
   - `gpt-4-32k`

3. **GPT-3.5 系列**
   - `gpt-3.5-turbo`

4. **其他厂商模型**
   - Anthropic Claude（`claude-3-5-sonnet-*`）
   - Google Gemini（`gemini-*`）
   - 所有国产模型（DeepSeek, 通义千问, 智谱等）

## 使用方法

### 判断模型是否使用新参数

```go
import "github.com/trailyai/traffic-ai/pkg/modelcompat"

if modelcompat.UsesMaxCompletionTokens("gpt-5.4") {
    // 使用 max_completion_tokens
} else {
    // 使用 max_tokens
}
```

### 获取正确的参数名

```go
paramName := modelcompat.TokenLimitParamName("gpt-4o-2024-11-20")
// 返回: "max_completion_tokens"

paramName := modelcompat.TokenLimitParamName("gpt-4")
// 返回: "max_tokens"
```

### 在实际请求中使用

```go
import (
    "encoding/json"
    "github.com/trailyai/traffic-ai/pkg/modelcompat"
)

reqBody := map[string]interface{}{
    "model":    modelName,
    "messages": messages,
}

// 根据模型选择正确的参数名
paramName := modelcompat.TokenLimitParamName(modelName)
reqBody[paramName] = maxTokens

bodyBytes, _ := json.Marshal(reqBody)
```

## 识别规则

### GPT-4o 日期判断

使用 **ISO 8601 日期格式** (YYYY-MM-DD) 进行字典序比较：

```
"2024-11-20" <= dateStr  // 返回 true 表示使用新参数
```

示例：
- `gpt-4o-2024-11-20` → `"2024-11-20" >= "2024-11-20"` → ✅
- `gpt-4o-2024-12-01` → `"2024-12-01" >= "2024-11-20"` → ✅
- `gpt-4o-2025-01-15` → `"2025-01-15" >= "2024-11-20"` → ✅
- `gpt-4o-2024-08-06` → `"2024-08-06" >= "2024-11-20"` → ❌

### GPT-5 识别

确保 `gpt-5` 后面不是数字（避免匹配 `gpt-50`, `gpt-500`）：

```go
// ✅ 匹配
gpt-5       // 后面没有字符
gpt-5.4     // 后面是 '.'
gpt-5-turbo // 后面是 '-'

// ❌ 不匹配
gpt-50      // 后面是数字 '0'
gpt-500     // 后面是数字 '0'
```

## 测试覆盖

运行测试：

```bash
go test -v ./pkg/modelcompat/
```

测试覆盖 30+ 个场景，包括：
- 新旧 GPT-4o 快照版本
- o1/o3 系列所有变体
- GPT-5 系列边界情况
- 其他厂商模型（Claude, Gemini, 国产模型）
- 大小写不敏感
- 边界情况（非官方命名、未来模型）

## 维护建议

当 OpenAI 发布新模型系列时，需要更新以下内容：

1. `token_params.go` - 添加新的识别规则
2. `token_params_test.go` - 添加对应测试用例
3. `README.md` - 更新文档说明

### 已知未来变更

- 如果 OpenAI 发布 GPT-6，需要添加类似 `gpt-5` 的判断逻辑
- 如果其他厂商（Anthropic/Google）也采用新参数，需要扩展规则
