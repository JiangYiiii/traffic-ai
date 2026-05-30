# 生产环境变量清单

traffic-ai 配置来源：**YAML 文件**（`configs/config.prod.yaml` 或挂载到 `/app/configs/config.yaml`）+ **环境变量覆盖**（优先级更高）。

两个进程 **control** 与 **gateway** 须使用**相同的** MySQL / Redis / JWT / AES 配置。

---

## 一、必配（上线前必须填写）

| 环境变量 | 适用服务 | 说明 | 示例 |
|----------|----------|------|------|
| `DB_HOST` | control + gateway | MySQL 主机（优先内网地址） | `10.0.0.5` |
| `DB_PORT` | control + gateway | MySQL 端口 | `3306` |
| `DB_USER` | control + gateway | MySQL 用户名 | `traffic_ai` |
| `DB_PASSWORD` | control + gateway | MySQL 密码 | `***` |
| `DB_NAME` | control + gateway | 数据库名 | `traffic_ai` |
| `REDIS_ADDR` | control + gateway | Redis 地址（`host:port`） | `10.0.0.6:6379` |
| `REDIS_PASSWORD` | control + gateway | Redis 密码（无密码可留空或不设） | `***` |
| `JWT_SECRET` | control + gateway | JWT 签名密钥，**随机强字符串** | `随机≥32字符` |
| `AES_KEY` | control + gateway | AES-256 密钥，**必须 32 字节**，加密上游 API Key | `32字节字符串` |

> control 与 gateway 的 `JWT_SECRET`、`AES_KEY` 必须完全一致，否则网关鉴权或凭证解密会失败。

---

## 二、端口（环境变量或 YAML 二选一）

| 环境变量 | YAML 字段 | 默认 | 适用服务 | 说明 |
|----------|-----------|------|----------|------|
| `CONTROL_PORT` | `server.control_port` | `8080` | control | 用户控制台 + 用户 API |
| `ADMIN_CONTROL_PORT` | `server.admin_control_port` | `8083` | control | 管理后台 + 管理 API |
| `GATEWAY_PORT` | `server.gateway_port` | `8081` | gateway | OpenAI 兼容 API |

**单端口模式**：`CONTROL_PORT` 与 `ADMIN_CONTROL_PORT` 设为**相同值**，用户控制台与管理后台合并为一个 HTTP 服务。

| 入口 | 路径 |
|------|------|
| 用户登录 | `/login.html` |
| 用户控制台 | `/app.html` |
| 管理登录 | `/admin-login.html` |
| 管理后台 | `/admin.html` |

**双端口模式（默认）**：用户 `8080`，管理 `8083`。

云托管端口映射须与容器内监听端口一致。

---

## 三、Gateway 运维开关（可选）

| 环境变量 | 默认值 | 说明 |
|----------|--------|------|
| `TRAFFIC_UPSTREAM_ENABLED` | `true` | 上游连接池总开关；`false` 降级为裸 `http.Client` |
| `TRAFFIC_CIRCUIT_ENABLED` | `true` | 账号级熔断总开关 |
| `TRAFFIC_UPSTREAM_LEGACY` | `0` | 设为 `1` 强制走 legacy 上游客户端（紧急回退） |
| `TRAFFIC_VERSION` | — | 仅用于 metrics 展示版本号，不影响业务 |

---

## 四、仅 YAML 配置（无环境变量覆盖）

以下项请在挂载的 `config.yaml` 中填写：

### 4.1 运行模式与日志（生产必改）

| YAML 路径 | 生产建议值 | 说明 |
|-----------|------------|------|
| `server.mode` | `release` | 关闭 Gin 调试输出 |
| `log.level` | `info` 或 `warn` | 日志级别 |
| `log.format` | `json` | 便于采集 |
| `log.output` | `stdout` | 容器标准输出 |

### 4.2 邮件（启用注册验证码发信时）

| YAML 路径 | 说明 |
|-----------|------|
| `email.smtp_host` | SMTP 服务器 |
| `email.smtp_port` | 默认 `587` |
| `email.username` / `email.password` | SMTP 凭证 |
| `email.from` | 发件人地址 |

未配置 SMTP 时，注册/重置验证码仅在服务端日志中输出（开发模式行为）。

### 4.3 OAuth 添加 OpenAI 模型（可选）

| YAML 路径 | 说明 |
|-----------|------|
| `oauth.public_base_url` | 管理后台对外 URL，如 `https://admin.example.com` |
| `oauth.providers.openai.client_id` | OpenAI OAuth Client ID |
| `oauth.providers.openai.client_secret` | OpenAI OAuth Client Secret |

回调地址：`{oauth.public_base_url}/admin/oauth/callback`

### 4.4 Gateway 性能调优（可选）

| YAML 路径 | 说明 |
|-----------|------|
| `gateway.upstream.*` | 连接池、超时（见 `docs/env-config.md`） |
| `gateway.circuit.*` | 熔断阈值、冷却时间 |
| `redis.db` / `redis.pool_size` | Redis 库编号与连接池 |
| `database.max_open_conns` 等 | MySQL 连接池 |

---

## 五、按服务汇总

### traffic-ai-control

```
必配: DB_* , REDIS_* , JWT_SECRET , AES_KEY
端口: CONTROL_PORT , ADMIN_CONTROL_PORT（可选）
YAML: server.mode=release , log.level=info , oauth.*（按需）
```

### traffic-ai-gateway

```
必配: DB_* , REDIS_* , JWT_SECRET , AES_KEY
端口: GATEWAY_PORT（可选）
开关: TRAFFIC_UPSTREAM_ENABLED , TRAFFIC_CIRCUIT_ENABLED , TRAFFIC_UPSTREAM_LEGACY
YAML: gateway.upstream.* , gateway.circuit.*（按需）
```

---

## 六、配置示例

### 6.1 环境变量（云托管控制台 / Secret）

见同目录 [`deploy/.env.example`](./.env.example)，复制为 `deploy/.env` 后填写。

```bash
# control + gateway 共用
DB_HOST=mysql.internal
DB_PORT=3306
DB_USER=traffic_ai
DB_PASSWORD=your-mysql-password
DB_NAME=traffic_ai

REDIS_ADDR=redis.internal:6379
REDIS_PASSWORD=your-redis-password

JWT_SECRET=your-random-jwt-secret-min-32-chars
AES_KEY=your-32-byte-aes-key-here!!!!!

# 单端口部署示例
CONTROL_PORT=8080
ADMIN_CONTROL_PORT=8080
GATEWAY_PORT=8081
```

### 6.2 挂载 YAML（与 claw_manager / Docker 一致）

```yaml
volumes:
  - /path/to/prod-config:/app/configs:ro
```

`prod-config/config.yaml` 中填写非敏感项与默认值；敏感项用环境变量注入覆盖。

---

## 七、相关文件

| 文件 | 用途 |
|------|------|
| `deploy/.env.example` | 环境变量模板 |
| `configs/config.prod.yaml.example` | 生产 YAML 模板 |
| `docs/env-config.md` | 全量配置项参考 |
| `docs/deploy.md` | 部署流程 |
