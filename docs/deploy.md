# 线上部署指南

本文描述 traffic-ai **控制面 + 数据面** 首次上线时的数据库初始化、配置与进程启动流程。

## 1. 架构与端口

| 进程 | 默认端口 | 说明 |
|------|----------|------|
| `control` | 8080（用户控制台）、8083（管理后台） | 认证、模型/路由、计费、静态前端 |
| `gateway` | 8081 | OpenAI 兼容 API 网关 |

## 2. 前置条件

- MySQL 8.x、Redis 6+
- 服务器可访问上游模型 API
- 已编译二进制：`make build` → `bin/control`、`bin/gateway`

## 3. 数据库初始化

### 3.1 一键脚本（推荐）

```bash
export DB_HOST=your-mysql-host
export DB_PORT=3306
export DB_USER=traffic_ai
export DB_PASSWORD='your-password'
export DB_NAME=traffic_ai

chmod +x scripts/deploy-init-db.sh
./scripts/deploy-init-db.sh
```

脚本会：

1. 创建 `traffic_ai` 库（若不存在）
2. 执行 `migrations/000001`～`000015` 全部 DDL（优先 `golang-migrate`，否则按序 `mysql` 导入）
3. 导入 `deploy/seed/001_baseline.sql`（默认 token 分组、全局并发兜底）

### 3.2 使用 golang-migrate 手动迁移

```bash
# 安装: brew install golang-migrate
export DSN="mysql://traffic_ai:password@tcp(127.0.0.1:3306)/traffic_ai?charset=utf8mb4&parseTime=True&loc=Local&multiStatements=true"
golang-migrate -path migrations -database "$DSN" up
mysql -h ... -u ... -p traffic_ai < deploy/seed/001_baseline.sql
```

### 3.3 Migration 清单

| 版本 | 说明 |
|------|------|
| 000001 | 基线表结构 + default token_group |
| 000002 | OAuth states |
| 000003 | endpoint /v1 修复 |
| 000004 | models 连通性测试字段 |
| 000005～000006 | 账号/套餐演进（套餐已移除） |
| 000007 | models.is_listed |
| 000008 | usage_logs 监控字段 |
| 000009 | usage_logs 扩展字段 |
| 000010 | 清理孤儿 upstream |
| 000011 | upstreams → model_accounts 重命名 |
| 000012 | usage_logs.error_message 加宽 |
| 000013 | model_accounts 连通性测试 |
| 000014 | embedding model_type 回填 |
| 000015 | Global AUTO 路由（virtual model / policy / candidate） |

> 本地开发若只需快速对齐旧库，仍可用 `scripts/sync-local-db.sh`（仅覆盖部分增量）；**生产请只用 `deploy-init-db.sh` 或 golang-migrate**。

## 4. 配置

```bash
cp configs/config.prod.yaml.example configs/config.prod.yaml
cp deploy/.env.example deploy/.env
# 编辑 config.prod.yaml 与 deploy/.env，填写 MySQL/Redis/密钥/域名
```

**生产必改项：**

| 配置 | 说明 |
|------|------|
| `jwt.secret` / `JWT_SECRET` | 随机强密钥 |
| `crypto.aes_key` / `AES_KEY` | 32 字节，用于加密上游凭证 |
| `database.password` | MySQL 强密码 |
| `redis.password` | Redis 密码（若有） |
| `server.mode` | `release` |
| `log.level` | `info` 或 `warn` |
| `oauth.public_base_url` | 管理后台对外 URL（若用 OAuth 添加 OpenAI） |

完整变量说明见 [env-config.md](./env-config.md)。

## 5. 创建管理员

数据库**不预置**用户密码（安全考虑）。首次上线：

1. 访问 `https://admin.your-domain.com/register.html` 或调用 API 注册
2. 提权为超级管理员：

```sql
UPDATE users SET role = 'super_admin' WHERE email = 'your-admin@example.com';
```

3. 在管理后台配置：模型 → 模型账号 → Token 分组 →（可选）AUTO 路由策略

## 6. 启动服务

```bash
# 加载环境变量（若使用 deploy/.env）
set -a && source deploy/.env && set +a

./bin/control -config configs/config.prod.yaml &
./bin/gateway -config configs/config.prod.yaml &
```

探活：

```bash
curl -s http://127.0.0.1:8080/healthz   # ok
curl -s http://127.0.0.1:8083/healthz   # ok
curl -s http://127.0.0.1:8081/healthz   # ok
```

## 7. 反向代理建议

| 路径 | 后端 |
|------|------|
| 用户控制台 | `control:8080` |
| 管理后台 | `control:8083` |
| `/v1/*` 网关 | `gateway:8081` |

OAuth 回调地址：`{oauth.public_base_url}/admin/oauth/callback`

## 8. 升级已有环境

```bash
# 仅执行增量 migration
golang-migrate -path migrations -database "$DSN" up

# 重新编译并滚动重启
make build
# 重启 control / gateway
```

## 9. 相关文件

| 路径 | 用途 |
|------|------|
| `scripts/deploy-init-db.sh` | 生产库初始化 |
| `deploy/seed/001_baseline.sql` | 基线配置数据 |
| `configs/config.prod.yaml.example` | 生产配置模板 |
| `deploy/.env.example` | 环境变量模板 |
| `migrations/` | 全部 DDL 版本 |
| `Dockerfile` | control / gateway 镜像构建 |
| `.cnb.yml` | CNB 云原生构建流水线 |

## 10. CNB 自动化构建

项目根目录 `.cnb.yml` 参考 hotel-management，在 **main / master** 分支 push 时自动：

1. 构建 `traffic-ai-control`、`traffic-ai-gateway` 两个镜像
2. 登录腾讯云 CCR（凭证经 `hotel-ccr-secrets` 密钥仓库 `imports` 注入）
3. 推送 `{commit}` 与 `latest` 双标签

### 10.1 首次配置

1. 在 [CNB](https://cnb.cool) 创建仓库（建议路径 `xlj.3jk/traffic-ai`），与 GitHub 同步或作为推送目标
2. 在腾讯云 CCR 创建两个镜像仓库：`traffic-ai-control`、`traffic-ai-gateway`
3. 编辑 `.cnb.yml` 中的 `CONTROL_IMAGE_BASE`、`GATEWAY_IMAGE_BASE` 为实际 CCR 地址
4. CCR 登录凭证沿用 `https://cnb.cool/xlj.3jk/hotel-ccr-secrets`（与 hotel-management 相同）

### 10.2 推送到 CNB

```bash
# 创建访问令牌: CNB → 个人设置 → 访问令牌（读写权限）
git remote add cnb https://cnb.cool/xlj.3jk/traffic-ai.git   # 首次
git push https://cnb:<CNB令牌>@cnb.cool/xlj.3jk/traffic-ai.git main
```

### 10.3 本地验证构建

```bash
chmod +x scripts/cnb-build-local.sh
./scripts/cnb-build-local.sh all
```

运行时请将生产配置挂载到 `/app/configs`（与 claw_manager 中 `~/openclaw-data/traffic-ai` 挂载方式一致），覆盖镜像内默认 `config.prod.yaml.example`。
