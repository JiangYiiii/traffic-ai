# 架构设计

## 整体架构

```
┌────────────────────┐  ┌───────────────┐  ┌───────────────────────┐
│      调用端         │  │ 客户端(控制台) │  │      管理后台          │
│ OpenClaw/Cursor/   │  │   app.html    │  │ 客户管理端 + 模型管理端 │
│ Cline/Continue     │  │   SPA 前端    │  │     admin.html        │
└────────┬───────────┘  └──────┬────────┘  └──────────┬────────────┘
         │ :8081               │ :8080                │ :8083
         ▼                     ▼                      ▼
┌────────────────┐    ┌──────────────────────────────────────┐
│  数据面网关     │    │           控制面 API                  │
│  Gateway       │    │  用户 API (:8080) + 管理 API (:8083) │
│  (无状态)       │    │  客户管理端: admin/super_admin        │
└────────┬───────┘    │  模型管理端: 仅 super_admin           │
         │            └──────────────┬───────────────────────┘
         │                           │
         ▼                           ▼
┌──────────────────────────────────────────┐
│            Redis (状态收敛)               │
│   余额缓存 · 限流计数 · 验证码            │
└──────────────────────────────────────────┘
┌──────────────────────────────────────────┐
│            MySQL (持久化)                 │
│   用户 · Key · 模型 · 流水 · 审计         │
└──────────────────────────────────────────┘
```

> **名词约定：** 详见 [glossary.md](glossary.md)。

## DDD 分层结构

```
cmd/
  control/main.go          # 控制面入口
  gateway/main.go          # 数据面入口

internal/
  domain/                  # 领域层 — 实体、接口、业务规则（无外部依赖）
    auth/                  # 用户认证
    token/                 # API Key
    billing/               # 余额计费
    model/                 # 模型管理（含 ModelAccount 模型账号实体）
    routing/               # 路由引擎
    ratelimit/             # 限流规则
    pkgplan/               # 套餐体系（Package/UserPackage）

  application/             # 应用层 — 用例编排（依赖 domain 接口）
    auth/usecase.go
    token/usecase.go
    billing/usecase.go
    model/usecase.go       # 模型 + 账号管理
    routing/usecase.go
    ratelimit/usecase.go
    gateway/usecase.go     # 网关全链路编排（含套餐校验）
    pkgplan/usecase.go     # 套餐 CRUD + 用户绑定

  infrastructure/          # 基础设施层 — 接口实现
    config/                # 配置加载
    persistence/mysql/     # MySQL 仓储实现 (10 个 repo)
    persistence/redis/     # Redis 缓存/限流/验证码

  interfaces/              # 接口层 — HTTP Handler
    api/                   # 控制面路由 + handler + dto + middleware
    gateway/               # 数据面路由 + 转发 handler

pkg/                       # 公共组件库（跨领域复用）
  aidoc/                   # @ai_doc 注解扫描系统
  crypto/                  # AES-256 加密 + bcrypt 哈希 + API Key SHA-256
  errcode/                 # 统一错误码 (10xxx~90xxx 分段)
  jwt/                     # JWT 双令牌 (access 2h + refresh 7d)
  logger/                  # zap 结构化日志
  response/                # 统一 JSON 响应格式
  httputil/                # RequestID 中间件
```

## 数据库设计 (16 张表)

| 表名 | 职责 |
|------|------|
| users | 用户账户 (邮箱/密码哈希/角色/状态) |
| api_keys | API Key (SHA-256 哈希存储，明文不落库；key_type 区分 standard/openclaw_token) |
| models | 模型定义 (名称/提供商/类型/定价) |
| model_accounts | 模型账号 (model_id → provider/name/endpoint/credential AES 加密/auth_type/protocol/weight/is_active) |
| token_groups | 令牌分组 (管理员级路由分流单元) |
| token_group_model_accounts | 分组-模型账号关联表 |
| packages | 套餐定义 (名称/描述/价格/有效天数/状态) |
| package_models | 套餐-模型关联表 (每个套餐包含哪些模型) |
| user_packages | 用户已购套餐 (用户→套餐绑定/有效期/状态) |
| user_balances | 用户余额 (microUSD 精度) |
| balance_logs | 余额流水 (充值/扣费/兑换/调账) |
| redeem_codes | 兑换码 |
| usage_logs | 调用日志 (请求链路完整记录) |
| audit_logs | 审计日志 (配置变更追踪) |
| rate_limit_rules | 限流规则 (四级: global/user/api_key/model) |

## 关键设计决策

### 控制面/数据面分离
- 数据面只做：鉴权 → 限流 → 路由 → 转发 → 计费，不承载复杂业务逻辑
- 控制面负责：用户管理、模型配置、策略管理、审计报表
- 控制面内部按角色分为**客户管理端**（`admin`/`super_admin`：充值、兑换码）和**模型管理端**（仅 `super_admin`：模型 CRUD、路由、限流）
- 数据面无状态，所有状态收敛至 Redis，可随时横向扩容

### 余额一致性
- Redis `DECRBY` 原子扣费保证并发安全
- 异步写 MySQL 流水保证持久化
- 定时对账任务比对两端数据（P2 实现）

### API Key 安全
- 明文仅创建时返回一次，服务端存 SHA-256 哈希
- 网关鉴权：调用端传入 Key → SHA-256 → 查 api_keys 表

### 流式 SSE 转发
- 非流式：读完整响应 → 解析 usage → 结算 → 返回
- 流式：逐 chunk Flush 透传 → 末尾 chunk 提取 usage → 结算

### 两层模型架构（模型 · 模型账号）
- **模型层 (models)**：对外的模型定义（名称、提供商、类型、计费方式、定价）
- **模型账号层 (model_accounts)**：模型到第三方 AI 服务的具体连接路径（provider、name、endpoint、credential、auth_type、protocol、weight、is_active）
- 同一模型可挂载多条模型账号，系统按权重与健康状态动态路由
- 历史上曾短暂拆分为 `provider_accounts` + `upstreams`；自 2026-04 起统一为 `model_accounts`（migration 000011）

### 套餐体系
- 套餐 (packages) 定义可用模型集合和定价
- 用户购买套餐后获得模型访问权限
- 网关校验顺序：API Key → 套餐校验（模型在套餐内?）→ tokenGroup 路由 → 动态选择账号
- 套餐与 tokenGroup 互补：套餐控制"能用哪些模型"（产品层），tokenGroup 控制"走哪些线路"（运维层）

### 路由引擎
- tokenGroup 作为管理员级分流单元，关联可用模型账号（model_accounts）
- 动态多因子路由：权重 × 模型账号剩余 QPS 余量 × 健康度
- 用户粘性路由：同一用户默认路由到同一模型账号（Redis user→model_account 映射，TTL 1h）
- **模型账号级熔断（P2D 落地）**：滑动窗口错误率 ≥ 50% 或连续 5 次 5xx/timeout 进入 `open`，首次冷却 30s；`half_open` 单探测成功后恢复 `closed`，失败则冷却指数退避到上限 5 分钟；状态存 Redis `circuit:account:{id}`（多实例共享），Redis 不可达时 fail-open 不阻塞业务
- **错误分类**：`timeout / 5xx / dial / tls` 计入熔断样本；`4xx`（用户错误）不计入，避免坏请求把账号熔掉
- **fallback 账号切换**：选中账号失败后自动在同 model 健康账号中再选最多 `max_retry_fallback` 次；流式响应一旦 `flusher.Flush()` 写出首字节即不再重试（避免客户端收到一半内容后被重置）
- 凭证保存在 `model_accounts.credential`，AES-256 加密存储，路由时解密替换

### 上游 HTTP 客户端池化（P2D 落地）
- 历史实现每次请求 `&http.Client{Timeout: ...}` 走 `http.DefaultTransport`（`MaxIdleConnsPerHost=2`），高并发下不停新建 TCP+TLS，是单账号吞吐瓶颈的根因
- 新实现 `internal/infrastructure/httpclient/manager.go` 按 `model_account.id` 缓存 `*http.Client`，每账号独立 `*http.Transport`：不同上游互不污染连接池，也便于按账号维度观测连接复用率
- 分项超时替代单一 `Client.Timeout`：`DialTimeout` / `TLSHandshakeTimeout` / `ResponseHeaderTimeout` / `StreamIdleTimeout` 各自独立（详见 env-config.md `gateway.upstream.*`）
- 流式 SSE 场景下不能用整次 `Client.Timeout`（会截断长对话）；改用每次 `reader.Read` 前重置 `SetReadDeadline` 实现"帧间空闲超时"
- 客户端断连（`c.Request.Context()` 取消）会立即 propagate 到上游请求，回收 goroutine 与连接
- 总开关 `gateway.upstream.enabled`（默认 true）+ 紧急回退环境变量 `TRAFFIC_UPSTREAM_LEGACY=1`，可随时退回老路径

### 账号级并发控制（P2D 落地）
- 现有限流 Scope: `global / user / api_key / model`，新增 **`account`** 作为第五级，直接保护上游不被单租户/单模型打爆
- 复用 `RateLimitRule.MaxConcurrent` + Redis `rl:concurrent:account:{id}` 计数，请求开始 `INCR`，`defer Release`
- **TPM 落地**：`RateLimitRule.MaxTPM` 字段原已有但未生效；现利用 Redis zset `rl:tpm:{scope}:{val}` 滑动窗口记录，预扣口径复用 `gateway/usecase.go` 的 `estimateCost`，`settleAndLog` 里按真实 usage 做差额修正，与计费口径完全一致不会漂移

### 可观测性（P2E 落地）
- **`/metrics`**：Prometheus 文本格式，基于 `github.com/prometheus/client_golang`；最少暴露 `traffic_gateway_requests_total{model,account,status,protocol}`、`traffic_gateway_upstream_latency_seconds{model,account}`（histogram）、`traffic_gateway_inflight{model,account}`、`traffic_gateway_circuit_state{account}`、`traffic_gateway_ratelimit_reject_total{scope,reason}`、`traffic_gateway_retry_total{model,reason}`
- **`/readyz`**：深度健康检查，`db.PingContext(30ms) + rdb.Ping(30ms)`，失败返 503；`/healthz` 保持轻量（只判断进程存活）
- **结构化日志字段**：`request_id, user_id, api_key_id, model, account_id, upstream_host, status, latency_ms, upstream_latency_ms, input_tokens, output_tokens, error_kind` 在 `settleAndLog` 里统一注入，方便按账号维度快速定位

### @ai_doc 注解体系
- `@ai_doc` — 标记业务含义
- `@ai_doc_flow` — 标记关键流程入口
- `@ai_doc_rule` — 标记业务规则
- `@ai_doc_edge` — 标记边界条件
- 通过 `pkg/aidoc.ScanDir()` 扫描提取全项目注解索引

## 启动方式

```bash
# 依赖
# MySQL: root@127.0.0.1:3306/traffic_ai (无密码)
# Redis: 127.0.0.1:6379

# 建库建表
mysql -u root -h 127.0.0.1 -e "CREATE DATABASE IF NOT EXISTS traffic_ai"
mysql -u root -h 127.0.0.1 traffic_ai < migrations/000001_init_schema.up.sql

# 启动控制面
go run ./cmd/control -config configs/config.yaml

# 启动数据面
go run ./cmd/gateway -config configs/config.yaml
```
