---
name: traffic-ai-upstream-endpoint-debug
description: 排查并修正 traffic-ai 中模型账号（model_accounts）上游接入地址 endpoint 配置；依据 DB 中 last_test_error、代码里 URL 拼接规则，用脚本批量探测候选 base。在用户反馈「账号连不通」「上游 404/400」「Fintopia/DashScope/OpenAI 兼容」或需核对接入地址时使用。
---

# traffic-ai 上游接入地址排查

## 代码事实（先对齐再猜）

- **探测与转发**均请求：`TrimRight(endpoint, "/") + "/chat/completions"`（OpenAI Chat 兼容）。
- 实现位置：`internal/application/model/usecase.go`（`testModelAccountHTTP`）、`internal/application/gateway/usecase.go`（上游 `upstreamURL`）。
- **权重**只做加权随机分流，与「能否连通」无关；见 `internal/application/routing/usecase.go` 的 `weightedRandom`。

因此：**接入地址必须是「OpenAI 兼容 base」**，不要手动再拼 `/chat/completions`（控制台/DB 里只存 base）。

## 第一步：读库看现网与错误原文

在 `model_accounts` 上查目标模型或账号名（按实际库名调整）：

```sql
SELECT ma.id, ma.model_id, m.model_name, ma.name, ma.endpoint, ma.is_active,
       ma.last_test_ok, ma.last_test_error, ma.last_test_at
FROM model_accounts ma
JOIN models m ON m.id = ma.model_id
WHERE m.model_name = '你的模型名' OR ma.name = '账号显示名';
```

- **`last_test_error` 为空且 `last_test_ok=1`**：当前配置至少通过了一次最小探测。
- **`HTTP 404`**：常见为 **base 少了一段路径**（例如缺 `v1` 或缺 `compatible-mode/v1`），实际打到了不存在的 path。
- **`HTTP 400` + 阿里云风格 JSON**（如 `InvalidParameter`、`url error, please check url`）：请求已进到「会转调 DashScope/百炼」的一层，多为 **网关侧拼给阿里云的 URL 不对**；在 traffic-ai 侧继续换 **与官方兼容路径同构的 base** 往往能解决。

## 第二步：对照「能通」的参考线

同一模型下若有一条 **直连阿里云** 且探测成功的账号，其 endpoint 一般为：

`https://dashscope.aliyuncs.com/compatible-mode/v1`

第三方网关若在域名下挂载「千问/OpenAI 兼容」，**路径层级常与上述一致**：在网关前缀后再接 **`/compatible-mode/v1`**，而不是仅 `/v1`。

## 第三步：批量探测候选 URL（推荐）

用与系统相同的 **POST 体**（与探测逻辑一致即可）：

```json
{"model":"<models.model_name>","messages":[{"role":"user","content":"Hi"}],"max_tokens":5}
```

Header：`Content-Type: application/json`，`Authorization: Bearer <明文 API Key>`。

对每个候选 **base**（无尾斜杠亦可）请求：

`{base}/chat/completions`

记录 **HTTP 状态码**与响应体前几百字：

- **2xx**：该 base 可作为 traffic-ai 的 `endpoint` 写入账号。
- **401 `Unknown model` / `Unauthorized token`**：多为网关路由层拒绝，说明 path 或 model 名与网关约定不一致；换 base 或核对网关文档。
- **400 且 DashScope 文案**：优先换与 `compatible-mode/v1` 同构的 base；若仍失败则属 **网关实现或 Key 权限**，需转内部网关维护方并带上 `request_id`。

**已知案例（Fintopia all-in-one-ai）**

- ❌ `https://all-in-one-ai.fintopia.tech/qwen` → 实际请求 `.../qwen/chat/completions` → **404**。
- ❌ `https://all-in-one-ai.fintopia.tech/qwen/v1` → **400** `url error`（下游 URL 仍不对）。
- ✅ `https://all-in-one-ai.fintopia.tech/qwen/compatible-mode/v1` → **200**（与阿里云兼容段对齐）。

可选：对同一网关 `GET {网关前缀}/v1/models`（若返回 200）可辅助确认网关是否采用类 OpenAI/DashScope 路由。

## 第四步：写回配置并复测

1. 在管理后台编辑该 **模型账号**，将 **接入地址** 改为探测成功的 **base**（整段 URL，勿含 `/chat/completions`）。
2. 调用「单账号连通性测试」或管理端批量测试；确认 `model_accounts.last_test_ok=1` 且 `last_test_error` 为空。
3. **勿在工单/聊天中粘贴生产 API Key**；排查后建议轮换已泄露的 Key。

## 快速检查清单

- [ ] 已确认实际请求 URL = `endpoint + "/chat/completions"`。
- [ ] 已查 `last_test_error` 区分 404（路径）与 400（业务/下游）。
- [ ] 已对「官方兼容路径」同构的多个 base 做 POST 对比。
- [ ] 修改后已复测 DB 或 API 中的 `last_test_*`。

## 延伸阅读（按需打开）

- 架构说明中与上游相关的章节：`docs/architecture.md`（若存在且仍准确则以仓库为准）。
