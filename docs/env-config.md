# 环境配置变量

配置文件: `configs/config.yaml`，支持环境变量覆盖（优先级高于 YAML）。

## 配置项总览

### server — 服务端口

| YAML 路径 | 默认值 | 说明 |
|-----------|--------|------|
| `server.control_port` | 8080 | 客户端（控制台）端口（用户 API + 登录/注册/控制台静态页） |
| `server.admin_control_port` | 8083 | 管理后台端口（客户管理端 + 模型管理端；管理员 API + `admin-login.html` / `admin.html`）；可用环境变量 `ADMIN_CONTROL_PORT` 覆盖 |
| `server.gateway_port` | 8081 | 数据面端口（调用端请求入口，网关转发） |
| `server.mode` | debug | 运行模式: `debug` / `release` |

### database — MySQL

| YAML 路径 | 环境变量覆盖 | 默认值 | 说明 |
|-----------|-------------|--------|------|
| `database.host` | `DB_HOST` | 127.0.0.1 | MySQL 地址 |
| `database.port` | — | 3306 | MySQL 端口 |
| `database.user` | — | root | 用户名 |
| `database.password` | `DB_PASSWORD` | (空) | 密码 |
| `database.name` | — | traffic_ai | 数据库名 |
| `database.max_open_conns` | — | 50 | 最大连接数 |
| `database.max_idle_conns` | — | 10 | 最大空闲连接 |
| `database.conn_max_lifetime` | — | 3600 | 连接最大生存时间(秒) |

### redis — Redis

| YAML 路径 | 环境变量覆盖 | 默认值 | 说明 |
|-----------|-------------|--------|------|
| `redis.addr` | `REDIS_ADDR` | 127.0.0.1:6379 | Redis 地址 |
| `redis.password` | — | (空) | 密码 |
| `redis.db` | — | 0 | DB 编号 |
| `redis.pool_size` | — | 20 | 连接池大小 |

### jwt — 认证令牌

| YAML 路径 | 环境变量覆盖 | 默认值 | 说明 |
|-----------|-------------|--------|------|
| `jwt.secret` | `JWT_SECRET` | (开发默认值) | 签名密钥，**生产必须更换** |
| `jwt.access_ttl` | — | 7200 | access_token 有效期(秒)，默认 2h |
| `jwt.refresh_ttl` | — | 604800 | refresh_token 有效期(秒)，默认 7d |

### crypto — 加密

| YAML 路径 | 环境变量覆盖 | 默认值 | 说明 |
|-----------|-------------|--------|------|
| `crypto.aes_key` | `AES_KEY` | (开发默认值) | AES-256 密钥(32字节)，加密上游凭证，**生产必须更换** |

### email — 邮件 (P1 未启用)

| YAML 路径 | 默认值 | 说明 |
|-----------|--------|------|
| `email.smtp_host` | (空) | SMTP 服务器 |
| `email.smtp_port` | 587 | SMTP 端口 |
| `email.username` | (空) | 登录用户名 |
| `email.password` | (空) | 登录密码 |
| `email.from` | noreply@4tk.ai | 发件人地址 |

### log — 日志

| YAML 路径 | 默认值 | 说明 |
|-----------|--------|------|
| `log.level` | debug | 日志级别: `debug` / `info` / `warn` / `error` |
| `log.format` | json | 输出格式: `json` / `text` |
| `log.output` | stdout | 输出目标: `stdout` / `file` |
| `log.file_path` | (空) | output=file 时的文件路径 |

### gateway — 数据面网关（P2D / P2E 新增）

数据面 gateway 专用配置段，控制上游转发的连接池、超时、熔断与并发上限。**所有字段均可选，缺失时使用下列默认值**；任一字段 0 视为使用默认。

#### gateway.upstream — 上游 HTTP 客户端

| YAML 路径 | 环境变量覆盖 | 默认值 | 说明 |
|-----------|-------------|--------|------|
| `gateway.upstream.enabled` | `TRAFFIC_UPSTREAM_ENABLED` | `true` | 总开关。**紧急回退**：设置 `TRAFFIC_UPSTREAM_LEGACY=1` 可强制回到老路径（裸 `http.Client{Timeout}`） |
| `gateway.upstream.max_idle_conns` | — | 256 | 全局最大空闲连接数（所有上游合计） |
| `gateway.upstream.max_idle_conns_per_host` | — | 64 | 单个上游 host 的最大空闲连接数（Go 默认仅 2，是高并发瓶颈的根因） |
| `gateway.upstream.max_conns_per_host` | — | 128 | 单个上游 host 的最大总连接数（硬上限，防打爆上游） |
| `gateway.upstream.idle_conn_timeout_sec` | — | 90 | 空闲连接多久后关闭 |
| `gateway.upstream.dial_timeout_sec` | — | 5 | TCP 连接建立超时 |
| `gateway.upstream.tls_handshake_timeout_sec` | — | 10 | TLS 握手超时 |
| `gateway.upstream.response_header_timeout_sec` | — | 30 | 从请求发出到收到响应头的等待上限；**直接决定上游 hang 时失败多快** |
| `gateway.upstream.stream_idle_timeout_sec` | — | 60 | 流式 SSE 两帧之间的最大静默，超过则断流 |

#### gateway.circuit — 账号级熔断器

| YAML 路径 | 默认值 | 说明 |
|-----------|--------|------|
| `gateway.circuit.enabled` | `true` | 熔断总开关；状态存 Redis `circuit:account:{id}` |
| `gateway.circuit.error_rate_threshold` | 0.5 | 触发 open 的错误率阈值（需同时满足 `min_samples`） |
| `gateway.circuit.min_samples` | 10 | 错误率计算的最小样本数，避免小样本抖动 |
| `gateway.circuit.window_sec` | 60 | 错误率滑动窗口长度 |
| `gateway.circuit.open_cooldown_sec` | 30 | 首次进入 open 的冷却时间 |
| `gateway.circuit.max_cooldown_sec` | 300 | 连续探测失败后冷却指数退避的上限 |
| `gateway.circuit.max_retry_fallback` | 2 | 单次请求最多 fallback 到多少个备用账号（0 = 不 fallback） |

#### gateway.concurrency — 并发硬上限（默认兜底）

DB 表 `rate_limit_rules` 里的具体规则优先级高于此处默认值；缺规则时使用下列兜底。

| YAML 路径 | 默认值 | 说明 |
|-----------|--------|------|
| `gateway.concurrency.default_account_max` | 32 | 单个 `model_account` 在途请求硬上限（新增 `Scope=account`） |
| `gateway.concurrency.default_model_max` | 100 | 单个对外 `model` 在途请求上限 |
| `gateway.concurrency.default_user_max` | 8 | 单用户在途请求上限 |
| `gateway.concurrency.global_max` | 500 | gateway 进程全局在途请求上限 |

### oauth — 管理后台「OAuth 授权」添加模型（OpenAI）

用于管理后台通过 OAuth 拉取/绑定 OpenAI 凭证。**默认 `client_id` / `client_secret` 为空**，未配置时授权链接会出现 `client_id=`，授权页无法正常打开；上线若需使用该能力，再按下列项补齐并重启控制面。

| YAML 路径 | 环境变量覆盖 | 默认值 | 说明 |
|-----------|-------------|--------|------|
| `oauth.public_base_url` | — | `http://127.0.0.1:8083` | 管理后台对外可访问的 **根 URL**（无尾斜杠）。生产须改为实际域名与协议，与负载/ingress 一致。 |
| `oauth.providers.openai.client_id` | — | (空) | OpenAI OAuth 应用的 Client ID，**必填**（否则无法授权）。 |
| `oauth.providers.openai.client_secret` | — | (空) | 同上 Client Secret；换 token 时使用，**必填**。 |
| `oauth.providers.openai.authorization_endpoint` | — | `https://auth.openai.com/authorize` | 一般保持默认。 |
| `oauth.providers.openai.token_endpoint` | — | `https://auth.openai.com/token` | 一般保持默认。 |
| `oauth.providers.openai.scopes` | — | `openid profile` | 一般保持默认。 |

**在 OpenAI 开发者侧注册 OAuth 应用时，回调地址（Redirect URI）必须为：**

`{oauth.public_base_url}/admin/oauth/callback`

示例：本机开发为 `http://127.0.0.1:8083/admin/oauth/callback`；生产为 `https://你的管理后台域名/admin/oauth/callback`。

**说明：** 当前 `internal/infrastructure/config/config.go` 中 **未** 对 OAuth 做环境变量覆盖，上线时请在 `configs/config.yaml`（或你们部署使用的等价配置源）中填写；若日后需要 `OAUTH_OPENAI_CLIENT_ID` 等变量，需另行在加载逻辑中增加映射。

## 快速启动

```bash
# 一键启动（自动建库建表 + 启动双服务）
bash scripts/dev-start.sh

# 查看状态
bash scripts/dev-start.sh status

# 停止
bash scripts/dev-start.sh stop

# 重启
bash scripts/dev-start.sh restart
```

## 生产环境必改项

| 配置 | 原因 |
|------|------|
| `jwt.secret` | 开发默认值不安全 |
| `crypto.aes_key` | 开发默认值不安全 |
| `server.mode` | 改为 `release` 关闭调试输出 |
| `database.password` | 设置强密码 |
| `redis.password` | 设置密码 |
| `log.level` | 改为 `info` 或 `warn` |
| `oauth.public_base_url` 与 `oauth.providers.openai.*` | 若使用管理后台 OAuth 添加 OpenAI 模型：`public_base_url` 与线上管理后台一致；填写有效的 `client_id` / `client_secret`；OAuth 应用回调填 `{public_base_url}/admin/oauth/callback` |
| `gateway.concurrency.global_max` 与 `gateway.upstream.max_conns_per_host` | 默认值按单机 16G 预算设定；生产应按实际上游账号数、平均请求大小、QPS 目标重新测算（见 dev-progress.md 的 Phase 0 诊断命令） |
