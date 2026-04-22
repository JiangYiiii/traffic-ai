# 自建 Embedding 经 Traffic 统一出口 — 设计说明与改造点

## 1. 背景与目标

### 1.1 背景

- 业务希望使用**自建向量模型**（部署在本地 **Podman** 等容器环境），避免将文本直发公网闭源 API，并控制算力与数据路径。
- 同时希望**对外仍只暴露 Traffic 数据面**（`POST /v1/embeddings` + 子令牌鉴权），统一**用量、余额、审计与价目**，与 Chat / Speech 等通道一致。

### 1.2 目标

| 维度 | 目标 |
|------|------|
| 协议 | 调用端仍使用 **OpenAI Embeddings 兼容** 请求体；不因自建而改变客户端集成方式（curl / SDK / OpenClaw 等）。 |
| 路由 | Traffic 网关将请求转发至 **自建服务** 的 OpenAI 兼容地址（通常为 `…/v1/embeddings`）。 |
| 计量 | **usage_logs、扣费、限流** 均在 Traffic 侧完成；Podman 仅负责推理与返回符合约定的 JSON。 |
| 运维 | Podman 生命周期、监控、证书与网络由基础设施侧维护；本文仅约束与网关对接的契约。 |

### 1.3 非目标（本期可不纳入）

- 在 Traffic 内嵌向量推理引擎或模型文件管理。
- 改造向量数据库或 RAG 业务链路（仅涉及「如何拿到 embedding」这一跳）。
- 必须支持官方 OpenAI **多模态 embedding** 的全部形态（若自建仅支持文本，可在文档中声明）。

---

## 2. 架构说明

```
调用端（OpenClaw / 自研服务 / curl）
        │  Authorization: Bearer <子令牌>
        │  POST {gateway}/v1/embeddings
        ▼
Traffic 数据面网关（鉴权 · 限流 · 余额预扣 · 路由 · 透传 · 结算 · usage_logs）
        │  Authorization: Bearer <上游密钥>
        │  POST {自建 Base URL}/v1/embeddings
        ▼
Podman 内 OpenAI 兼容 Embedding 服务（自建模型推理）
```

- **对外唯一入口**：Traffic 网关域名上的 `/v1/embeddings`。
- **对内上游**：Model Account 中配置的 **Endpoint** 指向 Podman 暴露的地址（如 `http://127.0.0.1:8088/v1` 或内网 VIP）；网关会将路径拼接为 `Endpoint + /embeddings`，与现有 Chat 拼接 `/chat/completions` 的方式一致。
- **模型名**：客户端 JSON 里的 `"model"` 必须与控制台中为该线路登记的 **模型名** 一致；该名称是**逻辑名**，可与开源权重名、官方 SKU 名不同。

---

## 3. 现状盘点（代码已实现部分）

以下能力**已在仓库中存在**，本次「接入自建 Embedding」多数为**配置与自建服务契约**，而非新增 HTTP 路由。

| 能力 | 位置 / 说明 |
|------|-------------|
| 数据面路由 | `internal/interfaces/gateway/router.go`：`POST /v1/embeddings` |
| Handler | `internal/interfaces/gateway/handler.go`：`Embeddings` → `genericProxy(..., "openai", "/embeddings")` |
| 转发与计费 | `internal/application/gateway/usecase.go`：`ProxyGeneric`（预扣、上游请求、非流式响应、`usage` 解析、结算、`usage_logs`） |
| 路由协议匹配 | `internal/application/routing/usecase.go`：`routingProtocolMatches` — 请求 `openai` 可与账号 `protocol=chat` 匹配 |
| 用量解析 | `internal/application/gateway/usage_parser.go`：OpenAI 家族含 `usage.prompt_tokens` 等，适用于典型 embedding 响应 |

**结论**：不因「自建」而必须新增一条 `/v1/embeddings`；核心是 **控制台配置模型 + Model Account + TokenGroup**，以及自建侧返回可被解析的 **`usage`**。

---

## 4. 改造点统计

### 4.1 配置与运维（必做）

| 序号 | 改造项 | 说明 |
|------|--------|------|
| C1 | Podman 部署自建推理服务 | 暴露 **OpenAI 兼容** `POST /v1/embeddings`；建议使用与 OpenAI 一致的请求/响应字段，便于透传与计费。 |
| C2 | 自建服务返回 `usage` | 响应 JSON 中包含 `usage`，且至少能通过现有解析链路映射到计费（典型：`prompt_tokens` → 输入 token）。若缺失，`usage_logs` 中 token 可能为 0，扣费依赖预扣与结算逻辑，易产生偏差，**强烈建议对齐**。 |
| C3 | 网络连通 | 运行 Traffic **网关**进程的主机须能访问 Podman 映射端口（本机 `127.0.0.1` 或内网）；防火墙与安全组放行。 |
| C4 | 控制台新建模型 | 新建一条 embedding 用模型：`model_name` = 对外暴露的逻辑名（如 `traffic-embed-bge`）；`model_type` 建议填 `embedding`（便于后续筛选展示）；定价（按 token / 按次）与上架状态按业务设定。 |
| C5 | Model Account | Endpoint = 自建服务的 OpenAI API **根路径**（例如 `http://host:port/v1`，无尾部斜杠亦可按现有拼接规则处理）；Credential 为自建侧鉴权用的 Bearer Token（若自建无鉴权，需在服务端或前置代理统一关闭外网访问，仅内网可调）。 |
| C6 | TokenGroup 绑定 | 将上述 Model Account 划入 API Key 使用的 **tokenGroup**，否则路由返回无可用线路。 |

### 4.2 文档与用户侧（建议）

| 序号 | 改造项 | 说明 |
|------|--------|------|
| D1 | `demo/userClient` / `web/console` 文案 | 在 Embeddings 小节补充：**上游可为自建兼容服务**；模型名以控制台为准；计费以 Traffic 为准。 |
| D2 | OpenClaw 同步脚本 | `scripts/openclaw-sync-traffic-models.py` 在同步 `models.providers.traffic` 时，会按 `TRAFFIC_EMBEDDING_MODEL` / 名称启发式 写入顶层 `embedding`（`apiKey` / `baseUrl` / `model` / `dimensions`），记忆检索即走网关 `POST /v1/embeddings`；自定义维度用 `TRAFFIC_EMBEDDING_DIMENSIONS`。 |
| D3 | `docs/project-doc.md` | 可选增量：Embeddings 一段增加「自建上游」脚注，避免读者误以为仅官方 API。 |

### 4.3 代码侧（按需，非必须）

| 序号 | 改造项 | 触发条件 | 预期改动方向 |
|------|--------|----------|----------------|
| E1 | `usage_logs.protocol` 区分 embedding | 监控上需按「embedding」过滤，与 chat 区分 | `ProxyGeneric` / `callCtx` 对 path `/embeddings` 标记 `protocol` 为 `embeddings`（注释中已提及该取值），并确认计费与展示无回归。 |
| E2 | 预扣金额 `estimateCost` | embedding 场景预扣偏多/偏少影响体验 | 对 `model_type == embedding` 或请求 path 使用更小默认 output 预估，或按 input 字符粗估。 |
| E3 | `GET /v1/models` 分类 | 不希望 chat 与 embedding 混排 | 增加 query 参数如 `type=embedding` 或响应中按 `model_type` 分组。 |
| E4 | 管理端「连通性测试」 | 仅限 chat 测试 | 为 embedding 模型增加「测试 Embeddings」按钮或文档说明用 curl 自测。 |
| E5 | 请求体大小上限 | 单条文本极大 | 评估是否单独提高 `/v1/embeddings` 的 body limit（当前与其它接口共用上限）。 |

---

## 5. 自建服务响应契约（建议）

为与现有网关行为一致，建议自建至少满足：

- HTTP `200` 时：`Content-Type: application/json`，Body 为合法 JSON。
- 包含 `object`、`data`（向量数组）、`model` 等与 OpenAI 类似的结构（**按需最小集**，以便客户端库不报错）。
- 包含 `usage` 对象，例如：

```json
"usage": {
  "prompt_tokens": 12,
  "total_tokens": 12
}
```

网关会用 OpenAI 家族解析逻辑将 `prompt_tokens` 映射为输入侧计费依据。

若自建使用完全不同的 JSON Schema，则需要**额外开发**网关侧的适配层（本期不计入「仅配置接入」范围）。

---

## 6. 风险与依赖

| 风险 | 缓解 |
|------|------|
| 自建无鉴权且映射到公网 | 仅绑定 `127.0.0.1` / 内网；或由 Nginx 侧做 mTLS / IP 白名单。 |
| `usage` 缺失导致计费不准 | 自建补齐 `usage`；或在 Traffic 侧做按次计费（`BillingPerRequest`）与运营对齐。 |
| 官方 SDK 与自建字段不完全一致 | 以最小 OpenAI 兼容子集为准，并在文档列出限制。 |

---

## 7. 验收标准

1. 使用子令牌调用 `POST {gateway}/v1/embeddings`，`model` 为控制台登记名，返回 **200** 且含 embedding 向量数据。
2. Traffic **usage_logs** 中可见对应记录；**扣费**与价目规则一致（或按次计费符合预期）。
3. 自建容器重启后，只要 Endpoint 可达，业务无协议层改造即可恢复。
4. （若实施 E1）监控或 SQL 可按 embedding 维度统计。

---

## 8. 小结

| 类别 | 工作量印象 |
|------|------------|
| 网关代码 | **基本无需新增端点**；可选增强见 §4.3。 |
| 主要工作 | **Podman 自建服务 + OpenAI 兼容 + usage + 控制台模型/账号/分组**。 |
| 文档 | 建议在 userClient/控制台说明「自建上游」与模型名约定。 |

---

## 9. 修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.0 | 2026-04-19 | 初稿：自建 Embedding + Traffic 统一出口与改造点清单 |
