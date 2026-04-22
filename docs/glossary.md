# 名词定义（Glossary）

本文档定义项目中四层核心角色名词，全项目文档与代码注释统一使用。

## 四层角色

| 名词 | 定义 | 技术实体 | 访问角色 |
|------|------|----------|----------|
| **调用端** | 调用流量控制系统 API 的服务或工具 | OpenClaw 多租户实例、Cursor、Cline、Continue、Dify 等；通过 API Key 鉴权走数据面网关 `:8081` | 持有有效 API Key 的任意服务 |
| **客户端（控制台）** | 用户购买模型流量、管理 API Key 的 Web 界面 | `app.html` + `/me/*` API；走控制面 `:8080` | `default` / `admin` / `super_admin` |
| **客户管理端** | 管理用户流量、费用与充值的后台模块 | `admin.html`「客户管理」Tab + `/admin/users/:id/charge`、`/admin/redeem-codes/*` API；走管理后台 `:8083` | `admin` / `super_admin` |
| **模型管理端** | 底层模型、路由策略与限流的管理模块 | `admin.html`「模型管理」Tab + `/admin/models/*`、`/admin/token-groups/*`、`/admin/rate-limits/*` API；走管理后台 `:8083` | 仅 `super_admin` |

## 角色与权限映射

| 角色 | 客户端（控制台） | 客户管理端 | 模型管理端 |
|------|:---:|:---:|:---:|
| `default`（普通用户） | ✅ | ❌ | ❌ |
| `admin`（普通管理员） | ✅ | ✅ | ❌ |
| `super_admin`（超级管理员） | ✅ | ✅ | ✅ |

## 端口与域名

| 服务 | 默认端口 | 域名（生产） | 说明 |
|------|---------|-------------|------|
| 客户端（控制台） | 8080 | `console.4tk.ai` | 用户注册、登录、API Key 管理、余额、日志 |
| 管理后台（客户管理端 + 模型管理端） | 8083 | 内部访问 | 管理员登录后按角色展示不同 Tab |
| 数据面网关 | 8081 | `api.4tk.ai` | 调用端发送模型请求的统一入口 |

## 新增核心概念

| 名词 | 定义 | 技术实体 |
|------|------|----------|
| **模型账号 (ModelAccount)** | 某个模型接到第三方 AI 服务的一条具体连接路径，包含提供商、凭证、端点、协议与权重 | `model_accounts` 表；一个模型可挂载多条模型账号，支持启停与权重调度 |
| **套餐 (Package)** | 定义一组可用模型的产品单元，用户购买套餐后获得模型访问权限 | `packages` + `package_models` 表；管理员在客户管理端创建和管理 |
| **用户套餐 (UserPackage)** | 用户与套餐的绑定关系，含有效期和状态 | `user_packages` 表；用户可同时持有多个套餐，可用模型取并集 |

## 两层模型架构

| 层级 | 实体 | 职责 | 关键属性 |
|------|------|------|----------|
| 模型层 | Model | 对外暴露的模型定义、提供商属性、价格 | model_name, provider, billing_type, 各项价格 |
| 账号层 | ModelAccount | 模型到第三方服务的连接方式 | model_id, provider, name, endpoint, credential, auth_type, protocol, weight, is_active |

> 历史上曾引入过 `provider_accounts`（账号层）+ `upstreams`（线路层）的两个实体拆分，但业务实际只需要一个「模型账号」抽象；自 2026-04 起两者统一为 `model_accounts`。

## 易混淆辨析

- **「调用端」≠「客户端」**：调用端是通过 API Key 调用模型网关的服务（如 OpenClaw 实例、IDE 插件）；客户端是用户在浏览器中操作的控制台。
- **「客户管理端」≠「模型管理端」**：客户管理端面向运营（充值、兑换码、套餐）；模型管理端面向技术管理（模型接入、账号、路由、限流）。二者共用 `admin.html` 页面，通过 Tab 和角色权限区分。
- **「套餐」≠「tokenGroup」**：套餐控制用户能使用哪些模型（产品层面），tokenGroup 控制模型走哪些模型账号（运维层面）。
- **「模型」≠「模型账号」**：模型是 `gpt-4o` 这样的对外定义，模型账号是访问该模型的某一条具体路径（凭证+端点+协议）。同一模型可有多条模型账号，系统按权重与健康状态动态路由。
- **「模型账号」和「上游（upstream）」**：代码中涉及 HTTP 请求的局部变量有时仍保留 `upstream` 作为通用网络术语，指「被调用的第三方 HTTP 服务」；领域模型层统一使用 `ModelAccount`。

## 网关运行期术语（P2D / P2E 新增）

以下术语用于描述 gateway 与上游交互过程中的性能/韧性机制，与 `model_accounts` 为粒度。

| 名词 | 定义 | 技术实体 / 关联 |
|------|------|----------------|
| **HTTP 客户端池（Upstream Client Pool）** | 按 `model_account.id` 维度复用的 `*http.Client` + `*http.Transport`，支持 KeepAlive 连接复用、每账号独立连接池 | `internal/infrastructure/httpclient/manager.go`（P2D 新增） |
| **分项超时（Segmented Timeouts）** | 在整次 `Client.Timeout` 之外，额外拆分 `DialTimeout` / `TLSHandshakeTimeout` / `ResponseHeaderTimeout` / `StreamIdleTimeout`，避免上游 hang 时硬等满总超时 | `gateway.upstream.*` 配置段（见 env-config.md） |
| **ResponseHeaderTimeout** | 从发出请求到收到上游响应头的等待上限（默认 30s）。**关键**：流式请求的"首字节超时"也由此控制 | `net/http.Transport.ResponseHeaderTimeout` |
| **Stream 空闲超时（StreamIdleTimeout）** | 流式转发时两帧 SSE 之间允许的最长静默（默认 60s）；超过则中断并让客户端重试 | `handleStream` 读循环中的 `SetReadDeadline` |
| **账号级熔断（Account Circuit Breaker）** | 以 `model_account.id` 为粒度的三态状态机：`closed` / `open` / `half_open`。滑动窗口错误率或连续失败达阈值进入 `open`，冷却后放单探测请求 | 状态存 Redis Hash `circuit:account:{id}`；逻辑在 `internal/application/routing/circuit.go`（P2D 新增） |
| **fallback 账号切换** | 路由选中账号后，若上游返回 5xx/timeout/dial error，自动在同 model 的其他健康账号中再选一次；受 `max_retry_fallback` 限制。**流式响应一旦 `flusher.Flush()`，不再重试** | `gateway/usecase.go` 重试包装层 |
| **账号级并发（ScopeAccount）** | 在现有 `global/user/api_key/model` 四级限流之上新增的第五级，限制单个 `model_account` 的在途请求数，保护上游不被单租户打爆 | `internal/domain/ratelimit/entity.go` 新增 `Scope=account` |
| **TPM 限流（Tokens Per Minute）** | 滑动 60s 窗口内累计的 token 数上限。预扣口径复用 `estimateCost`，`settleAndLog` 里做真实消耗修正 | Redis zset `rl:tpm:{scope}:{val}`；利用 `RateLimitRule.MaxTPM` 已有字段 |
| **上游错误分类（UpstreamError.Kind）** | 网关层对上游失败的统一归类：`timeout / 5xx / 4xx / dial / tls`。4xx 归为客户端错误**不计入熔断样本**，避免坏请求连带把账号熔了 | `internal/application/gateway/upstream_error.go`（P2D 新增） |
| **/metrics** | Prometheus 文本格式的指标端点，挂在 gateway `:8081/metrics` | `github.com/prometheus/client_golang`（P2E 新增） |
| **/readyz** | 深度健康检查：`db.PingContext` + `rdb.Ping`，失败返回 503。`/healthz` 保持轻量（仅进程存活） | gateway + control 两侧均有（P2E 新增） |
