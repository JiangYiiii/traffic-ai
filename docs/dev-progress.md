# 开发进展记录

## P1 基础功能搭建 — 已完成 (2026-04-15)

### 交付概况

| 指标 | 数值 |
|------|------|
| Go 文件 | 65 |
| Go 代码行 | ~6000 |
| SQL 代码行 | 220 |
| 数据库表 | 12 张 |
| 独立服务 | 3 个（客户端控制面 :8080 + 管理后台 :8083 + 数据面 :8081） |

### 已完成模块

| 阶段 | 模块 | 状态 |
|------|------|------|
| P1A | 项目骨架 + Go Module + DDD 目录 + Config + Makefile | ✅ |
| P1A | 数据库 Schema + 迁移脚本 (12 张表) | ✅ |
| P1A | 核心组件库 pkg/ (aidoc, crypto, errcode, jwt, logger, response, httputil) | ✅ |
| P1B | Auth 领域 (邮箱注册/登录/JWT 双令牌/密码重置) | ✅ |
| P1B | API Key 领域 (创建/列表/启禁用/删除) | ✅ |
| P1C | Model & Routing 领域 (模型 CRUD/上游线路/tokenGroup 路由) | ✅ |
| P1C | Gateway 数据面 (OpenAI Chat Completions 转发/SSE 流式) | ✅ |
| P1D | Balance & Billing 领域 (微美元余额/Redis 原子扣费/兑换码) | ✅ |
| P1D | Rate Limiting 领域 (Redis 滑动窗口/四级限流) | ✅ |
| P1E | 控制面路由组装 + JWT/Admin 中间件 | ✅ |

### P1 已验证的 API 端点

```
# 认证（无需 JWT）
POST /auth/register/send-code
POST /auth/register
POST /auth/login
POST /auth/refresh
POST /auth/reset-password/send-code
POST /auth/reset-password

# 客户端 / 控制台（需 JWT）
GET  /account/profile
GET  /me/tokens
POST /me/tokens
PATCH /me/tokens/:id/disable
PATCH /me/tokens/:id/enable
DELETE /me/tokens/:id
GET  /me/balance/logs
POST /me/balance/redeem
PATCH /me/balance-alert

# 客户管理端（需 JWT + admin/super_admin）
POST /admin/users/:id/charge
POST /admin/redeem-codes/batch
GET  /admin/redeem-codes

# 模型管理端（需 JWT + super_admin）
GET/POST/PUT/DELETE /admin/models
GET/POST           /admin/models/:id/model-accounts   (兼容别名 /upstreams)
PUT/DELETE         /admin/model-accounts/:id          (兼容别名 /upstreams/:id)
GET/POST           /admin/token-groups
POST/DELETE        /admin/token-groups/:id/model-accounts (兼容别名 /upstreams)
GET/POST/PUT/DELETE /admin/rate-limits

# 网关数据面（API Key 鉴权）
GET  /v1/models
POST /v1/chat/completions  (支持 stream:true)

# 系统
GET /healthz
GET /readyz
```

---

## P2A 核心功能补齐 — 已完成 (2026-04-16)

### 交付概况

| 指标 | 数值 |
|------|------|
| Go 文件 | 73 |
| Go 代码行 | ~7000 |
| 新增后端 API | 4 个 |
| 新增前端功能 | 5 个（模型测试、使用日志、用户列表、充值记录、Playground） |
| 本地开发启动 | `scripts/dev-start.sh`（本机 MySQL + Redis） |

### 已完成模块

| 任务 | 模块 | 状态 |
|------|------|------|
| T1 | 模型管理端 - 模型连通性测试 (`POST /admin/models/:id/test`) | ✅ |
| T2 | 模型管理端 - 使用量日志查看 (`GET /admin/usage-logs`) | ✅ |
| T3 | 客户管理端 - 用户列表 (`GET /admin/users`) | ✅ |
| T4 | 客户管理端 - 全站余额流水 (`GET /admin/balance-logs`) | ✅ |
| T5 | 客户端控制台 - Playground 通路测试 | ✅ |
| T6 | 本地开发脚本部署 (MySQL+Redis+控制面+网关) | ✅ |

### P2A 新增 API 端点

```
# 模型管理端（需 JWT + super_admin）
POST /admin/models/:id/test     模型连通性测试（向上游发轻量请求）
GET  /admin/usage-logs           使用日志分页查询（支持 model/status 筛选）

# 客户管理端（需 JWT + admin/super_admin）
GET  /admin/users                用户列表分页查询（支持 email 模糊搜索）
GET  /admin/balance-logs         全站余额流水分页查询（支持 reason_type 筛选）
```

### P2A 前端功能

| 端 | 功能 | 说明 |
|-----|------|------|
| 模型管理端 | 模型连通测试按钮 | 每个模型行增加"测试"按钮，调用上游验证连通性 |
| 模型管理端 | 使用日志查看 | 全站调用日志，支持按模型/状态筛选、分页 |
| 客户管理端 | 用户列表 | 全站用户列表，支持邮箱搜索，快捷跳转充值 |
| 客户管理端 | 充值记录 | 全站余额流水，支持按变动类型筛选、分页 |
| 客户端控制台 | Playground | 选择 Key+模型发送测试消息，支持流式/非流式，验证全链路 |

### 部署方式

```bash
./scripts/dev-start.sh start
# 控制面: http://localhost:8080 (用户) / http://localhost:8083 (管理)
# 网关:   http://localhost:8081
```

### 全链路验证路径

1. 管理端 (:8083) 登录 → 添加模型 + 上游线路 → 测试连通 → 配置 tokenGroup
2. 客户管理端 → 查看用户 → 给用户充值 → 查看充值记录
3. 客户端 (:8080) 登录 → 创建 API Key → Playground 发送测试 → 确认模型返回
4. 将 API Key 配置到 OpenClaw (`base_url=http://localhost:8081`, `api_key=sk_xxx`) → 实际调用

---

## P2B 架构升级 — 已完成 (2026-04-16)

### 交付概况

| 指标 | 数值 |
|------|------|
| Go 文件 | 83+ |
| Go 代码行 | ~9000 |
| 新增数据库表 | 3 张 (packages, package_models, user_packages) |
| 修改数据库表 | 2 张 (upstreams → model_accounts 改名并收敛 provider 字段, api_keys +key_type) |
| 新增后端 API | 15+ 个 |
| 新增前端功能 | 3 个（账号管理、套餐管理、用户套餐面板） |

### 已完成模块

| 阶段 | 模块 | 状态 |
|------|------|------|
| S1 | 数据库迁移 — packages/package_models/user_packages + upstreams/api_keys 改造 | ✅ |
| S2 | 模型账号管理 — ModelAccount 域层实体/仓储 + MySQL Repo + UseCase + Handler/DTO/路由注册 | ✅ |
| S2 | Model · ModelAccount 两层架构收敛（废弃 provider_accounts，upstreams → model_accounts，migration 000011） | ✅ |
| S3 | 套餐体系 — Package/PackageModel/UserPackage 全栈实现 | ✅ |
| S3 | 用户端"我的套餐" — API + 前端展示 | ✅ |
| S4 | 路由引擎 — 多因子路由架构预留（account_id 贯穿） | ✅ |
| S5 | 网关套餐校验 — ChatCompletions 链路增加 package 检查 | ✅ |
| S5 | API Key key_type — 区分 standard / openclaw_token | ✅ |
| S5 | 前端 — admin.html 账号管理+套餐管理 section、app.html 套餐面板 | ✅ |

### P2B 新增 API 端点

```
# 模型管理端 — 提供商账号 CRUD（需 JWT + super_admin）
GET    /admin/accounts              账号列表（支持 provider/status/q 筛选）
POST   /admin/accounts              创建账号
PUT    /admin/accounts/:id          更新账号
PATCH  /admin/accounts/:id/status   上线/下线
DELETE /admin/accounts/:id          删除（软删除）

# 客户管理端 — 套餐管理（需 JWT + admin/super_admin）
GET    /admin/packages              套餐列表
POST   /admin/packages              创建套餐
PUT    /admin/packages/:id          更新套餐
DELETE /admin/packages/:id          删除套餐
PUT    /admin/packages/:id/models   设置套餐包含的模型
GET    /admin/packages/:id/models   查看套餐模型列表
POST   /admin/users/:id/packages    为用户绑定套餐
DELETE /admin/users/:id/packages/:upid  取消用户套餐
GET    /admin/users/:id/packages    查看用户套餐列表

# 客户端 — 我的套餐（需 JWT）
GET    /me/packages                 我的活跃套餐
GET    /me/packages/models          我的可用模型 ID 列表
```

### 架构变化

| 变化 | 说明 |
|------|------|
| 两层模型架构 | Model(定价) → ModelAccount(连接第三方服务的凭证/端点/协议/权重)；统一取代历史上的 ProviderAccount+Upstream 拆分 |
| 套餐体系 | Package ↔ Model 多对多，UserPackage 绑定用户，网关校验模型访问权限 |
| API Key 类型 | key_type 字段区分 standard/openclaw_token，为 OpenClaw 集成预留 |
| 网关校验链 | AuthMiddleware → RateLimit → Route → **PackageCheck** → Balance → Proxy |

---

## P2D + P2E: 高并发韧性与可观测性 — 进行中 (2026-04-20 启动)

### 触发事件

2026-04-20 14:37–16:20 Traffic 网关对所有模型持续返回 `502 (no body)`，单次请求客户端等待约 240s 才超时；openclaw 侧累计记录 502 共 257 次。**根因盘点**（见方案文档）：

- `internal/application/gateway/usecase.go:178` 每次请求新建 `http.Client`，`Transport=nil` 走 `http.DefaultTransport`，`MaxIdleConnsPerHost` 默认仅 2 → 高并发不停新建 TCP+TLS
- 只有整次 `Client.Timeout`（默认 120s），**无 `DialContext` / `ResponseHeaderTimeout`** → 上游 hang 就硬等满 120s
- `weightedRandom` 选账号后 **无失败切换 / 无熔断** → 坏账号会被反复命中
- `RateLimitRule.MaxTPM` 字段已存在但限流器未实现，**无账号级并发上限**
- 全仓库无 `/metrics`，`/healthz` 只返回 `ok` 不 ping DB/Redis，排障靠日志

### 总体拆分

| Phase | 目标 | 状态 | 关键交付 |
|-------|------|------|---------|
| **Phase 0** 诊断 | 量化当前连接池/DB/Redis/model-account 拓扑/错误率基线 | 🔄 即将开工 | 6 条可直接执行的 `lsof / mysql / redis-cli` 命令，输出结果作为 Phase 2/3 阈值依据 |
| **Phase 5.1/5.2** 可观测性基建 | 先把仪表装上，后续调参有依据 | 🔄 即将开工 | `/metrics` (Prometheus) + `/readyz` (DB+Redis ping)；新增 go.mod 依赖 `github.com/prometheus/client_golang` |
| **Phase 1** 连接池 + 分项超时 | **直接修复 4-20 事故模式** | 🔄 即将开工 | `internal/infrastructure/httpclient/manager.go`（按 account_id 缓存 `*http.Client`）；替换 `usecase.go:178` 的裸 Client |
| **Phase 2** 账号级熔断 + fallback | 下次上游抖动时秒级自动切换 | ⏳ 待 Phase 1 落地 | `internal/application/routing/circuit.go`（Redis Hash 状态机）；`SelectModelAccount` 过滤 open；`ChatCompletions/AnthropicMessages/ProxyGeneric` 包重试层 |
| **Phase 3** 账号并发 + TPM | 保护上游不被打爆 | ⏳ 待 Phase 2 落地 | `ratelimit.Scope` 新增 `account`；Redis `rate_limiter` 落地 TPM（zset 滑动窗口，预扣复用 `estimateCost`） |
| **Phase 5.3/5.4** 日志字段 + 告警模板 | 统一可观测性字段 | ⏳ 穿插进行 | `settleAndLog` 结构化字段补齐；docs 补 Prometheus 告警规则模板 |
| **Phase 3.4** 队列深度（可选） | 满并发时短时排队 | ⏳ 二期 | `RateLimitRule.QueueDepth` / `QueueWaitMs`；超时返回 503 + Retry-After |
| **Phase 4** 客户端集成文档 | openclaw 侧配 fallback provider | ⏳ 二期 | `docs/client-integration.md` |

### 已拍板决策（锁定，开发中不再反复讨论）

| 决策点 | 选择 | 依据 |
|--------|------|------|
| 熔断状态存储 | **Redis Hash** `circuit:account:{id}`，Lua 合并读写 RTT | 支持未来多实例横向扩容，单机增量 <1ms/请求 |
| 熔断 Redis 不可达行为 | **fail-open**（视为所有账号健康，不阻塞业务）+ 日志 Warn + metric 计数 | 熔断器失效远比阻塞业务代价小 |
| TPM 预估口径 | **复用 `gateway/usecase.go` 的 `estimateCost`** | 限流/计费三者同源，零双实现风险 |
| 流式重试边界 | **进入 `handleStream` 且 `flusher.Flush()` 后不再重试**；首字节前失败可走 fallback | 避免客户端收到一半内容后重置 |
| Metrics 方案 | **`github.com/prometheus/client_golang`**，`/metrics` 用 `promhttp.Handler()` | 行业标配，Grafana 零成本对接 |
| Phase 1 首次上线姿势 | `gateway.upstream.enabled: true` **默认即生效** + 紧急回退环境变量 `TRAFFIC_UPSTREAM_LEGACY=1` | 4-20 事故模式最快修复；保留秒级一键回退 |

### 协议兼容性约束

- **不改变 openai-completions / anthropic messages / gemini generateContent 的对外行为**；所有优化在"gateway 内部如何转发"这一侧发生
- 新增 `/metrics` 与 `/readyz` 不挂 `AuthMiddleware`，与 `/healthz` 同级处理
- 新增 `gateway:` 配置段所有字段均可选，缺失退回默认值；老部署 yaml 不改也能启动

### 相关文档

- 架构设计与数据流：[architecture.md](architecture.md)（"路由引擎"、"上游 HTTP 客户端池化"、"账号级并发控制"、"可观测性" 四段）
- 配置项：[env-config.md](env-config.md)（`gateway.upstream` / `gateway.circuit` / `gateway.concurrency` 三段）
- 术语：[glossary.md](glossary.md) "网关运行期术语" 段

---

## 后续推进计划（尚未排期）

### P2C: 多协议扩展

| 优先级 | 模块 | 说明 |
|--------|------|------|
| P2C-1 | Anthropic Messages 协议 | `/v1/messages`，`x-api-key` 鉴权适配 |
| P2C-2 | Gemini 协议 | `/v1beta/models/{model}:generateContent` + streamGenerateContent |
| P2C-3 | OpenAI Embeddings / Audio Speech | `/v1/embeddings` + `/v1/audio/speech` |
| P2C-4 | OpenAI Responses API | `/v1/responses`，protocol=responses 的模型账号 |

### P2D 剩余项（本次未覆盖）

| 优先级 | 模块 | 说明 |
|--------|------|------|
| P2D-1 | 用户粘性路由 | Redis user→account 映射，同用户默认路由到同一账号（与本次熔断解耦） |
| P2D-3 | 多因子打分路由 | 权重 × QPS 余量 × 健康度动态选择（本次只做了健康度过滤） |

### P2E 剩余项（本次未覆盖）

| 优先级 | 模块 | 说明 |
|--------|------|------|
| P2E-2 | 调用日志导出 | CSV 导出 + 管理后台链路追踪 |
| P2E-3 | Grafana 大屏 Dashboard | 基于本次新增的 Prometheus 指标做标准看板 |

### P3: 高级能力

| 模块 | 说明 |
|------|------|
| OpenClaw 完整集成 | openclaw_token 全链路打通，容器隔离+自动套餐绑定 |
| 四级降级策略 | 同厂商换 Key → 降级模型 → 跨厂商替代 → 友好错误（本次的 fallback 是其中第 1 级） |
| Clash IP 池调度 | 对接 Clash REST API，轮询/哈希/故障切换 |
| 精确缓存引擎 | 相同 prompt+参数 SHA 命中，本地+Redis 二级缓存 |
| 凭证自动轮换 | 到期预警 → 灰度验证 → 自动切流 |
| 邮件服务集成 | SMTP 发送验证码 + 余额提醒 |
| 成本可视化看板 | 月度总花费、模型占比、成员排行 |
| SSE 实时推送 | 控制台余额/Key 状态变化推送 |
| i18n 完善 | 中英文切换全覆盖 |
