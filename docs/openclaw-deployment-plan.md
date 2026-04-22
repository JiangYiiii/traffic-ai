# OpenClaw 多租户部署方案（腾讯云 · 流量网关集成）

> 版本：v0.1 · 2026-04-15
> 状态：方案设计阶段
> 目标：为 ~10 位用户部署共享 OpenClaw 服务，各租户全隔离，模型统一接入流量控制系统

---

## 一、背景与目标

### 1.1 OpenClaw 简介

[OpenClaw](https://openclaw.ai/) 是一个开源个人 AI 助手，由 Peter Steinberger 创建。核心特性：

- 运行在用户自己的机器上，支持 macOS / Linux / Windows
- 通过 WhatsApp / Telegram / Discord / Slack / iMessage 等聊天应用交互
- 持久记忆、浏览器控制、文件系统访问、Shell 执行
- Skills 插件体系（ClawHub 社区已有 13,000+ 技能）
- MCP（Model Context Protocol）协议扩展工具能力
- 支持 Anthropic / OpenAI / Gemini / 本地模型等多种 LLM 提供商
- 网关绑定单端口（默认 18789），暴露 OpenAI 兼容端点（`/v1/chat/completions` 等）

### 1.2 部署目标

| 维度 | 目标 |
|------|------|
| 用户规模 | ~10 位用户，每人一个独立 OpenClaw 实例 |
| 租户隔离 | 逻辑隔离、Skill 隔离、上下文/记忆隔离、MCP 隔离、角色预设隔离 |
| 模型接入 | 统一通过本项目流量控制网关（`api.4tk.ai`），每租户自动分配独立 API Key |
| 多角色 | 每个租户可定制多个 AI 角色/Persona，适配不同场景 |
| Skill 池 | 共享 Skill 仓库，用户可搜索、安装已开放的现成功能 |
| 部署环境 | 腾讯云，后续迁移至阿里云 |
| 接入方式 | 用户通过 Telegram / Discord / Web 与自己的 OpenClaw 实例交互 |

> **在流量控制系统中的定位：** OpenClaw 多租户实例属于**调用端**角色（详见 [glossary.md](glossary.md)），每个租户通过独立 API Key 经 tokenGroup `openclaw-tenants` 接入数据面网关 `:8081`，共享同一套模型路由、限流与计费体系。OpenClaw 支持多租户，多个租户之间使用不同的 API Key 调用模型，互不影响。

---

## 二、整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        用户接入层                                │
│   Telegram Bot / Discord Bot / Web Dashboard / WhatsApp         │
└──────────────────────────┬──────────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────────┐
│                   Nginx 反向代理 + TLS                           │
│           claw.4tk.ai (统一入口 · 路径/子域路由)                  │
│   ┌──────────────┬──────────────┬──────────────────────┐        │
│   │ /admin/*     │ /api/*       │ /tenant-{id}/*       │        │
│   │ 管理后台     │ 租户管理 API  │ 各租户 OpenClaw 网关  │        │
│   └──────┬───────┴──────┬───────┴──────────┬───────────┘        │
└──────────┼──────────────┼──────────────────┼────────────────────┘
           │              │                  │
┌──────────▼──────────────▼──────────────────▼────────────────────┐
│                     Docker Compose 集群                          │
│                                                                  │
│  ┌─────────────────┐  ┌──────────────────────────────────────┐  │
│  │ 租户管理服务     │  │ OpenClaw 容器池 (每租户一个)          │  │
│  │ (tenant-mgr)    │  │                                      │  │
│  │                 │  │  ┌────────┐ ┌────────┐ ┌────────┐   │  │
│  │ - 用户注册登录  │  │  │User-A  │ │ User-B │ │ User-C │   │  │
│  │ - 租户开通/注销 │  │  │        │ │        │ │        │   │  │
│  │ - API Key 分发  │  │  │ Skills │ │ Skills │ │ Skills │   │  │
│  │ - Skill 池管理  │  │  │ Memory │ │ Memory │ │ Memory │   │  │
│  │ - 角色模板管理  │  │  │ MCP    │ │ MCP    │ │ MCP    │   │  │
│  │ - 监控与日志    │  │  │ Roles  │ │ Roles  │ │ Roles  │   │  │
│  └────────┬────────┘  │  └────────┘ └────────┘ └────────┘   │  │
│           │           └──────────────────────────────────────┘  │
│           │                                                      │
│  ┌────────▼────────┐  ┌──────────────────┐                      │
│  │ PostgreSQL      │  │ Skill 仓库服务    │                      │
│  │ (租户元数据)    │  │ (skill-registry) │                      │
│  └─────────────────┘  └──────────────────┘                      │
└──────────────────────────────┬───────────────────────────────────┘
                               │
              ┌────────────────▼────────────────┐
              │   流量控制网关 (api.4tk.ai)       │
              │   本项目 AI 模型流量控制系统       │
              │                                  │
              │   - API Key 认证                 │
              │   - 模型路由 / 限流 / 降级        │
              │   - 计费扣费                     │
              │   - IP 池调度                    │
              └──────────────────────────────────┘
                               │
              ┌────────────────▼────────────────┐
              │   上游 AI 模型厂商                │
              │   OpenAI / Anthropic / Gemini    │
              └─────────────────────────────────┘
```

### 2.1 核心设计决策

| 决策点 | 方案 | 理由 |
|--------|------|------|
| 多租户隔离方式 | **容器级隔离**（每租户独立 Docker 容器） | OpenClaw 原生为单用户设计，容器隔离是最干净的隔离方式；10 用户规模资源可控 |
| 模型接入 | 所有容器统一指向 `api.4tk.ai`，各自持有独立 API Key | 复用流量控制系统的计费、限流、降级、监控能力 |
| Skill 共享 | 中心化 Skill 仓库 + 租户侧按需安装 | 管理员统一维护可用 Skill 池，用户自助搜索安装 |
| 角色管理 | 每租户独立 Persona SKILL.md 文件 + 管理后台模板库 | 灵活度高，用户可自定义也可从模板选用 |
| MCP 隔离 | 每容器独立 `openclaw.json5` MCP 配置 | 容器级天然隔离，互不影响 |
| 持久化 | 每容器独立 Volume 挂载 `~/.openclaw/` 目录 | 记忆、Skills、配置完全隔离，容器重建不丢数据 |

---

## 三、租户隔离设计

### 3.1 隔离维度矩阵

| 隔离维度 | 实现方式 | 存储位置 |
|----------|----------|----------|
| **逻辑隔离** | 每租户独立 Docker 容器 + 独立网络命名空间 | Docker Network |
| **Skill 隔离** | 每容器独立 `~/.openclaw/skills/` 挂载目录 | Volume: `/data/tenants/{id}/skills/` |
| **上下文/记忆隔离** | 每容器独立 `~/.openclaw/memory/` 目录 | Volume: `/data/tenants/{id}/memory/` |
| **MCP 隔离** | 每容器独立 `openclaw.json5` 配置文件 | Volume: `/data/tenants/{id}/config/` |
| **角色预设隔离** | 每容器 `~/.openclaw/skills/personas/` 下的角色 SKILL.md | Volume: `/data/tenants/{id}/skills/personas/` |
| **凭证隔离** | 每容器独立 `.env` 文件，仅注入该租户的 API Key | Volume: `/data/tenants/{id}/env/` |
| **资源隔离** | Docker `--cpus` / `--memory` 限制 | docker-compose resource limits |

### 3.2 容器资源限制（单租户）

```yaml
deploy:
  resources:
    limits:
      cpus: '1.0'
      memory: 1G
    reservations:
      cpus: '0.25'
      memory: 256M
```

### 3.3 网络隔离

```
┌─ claw-network (bridge) ──────────────────────────────┐
│                                                       │
│  nginx ◄─► tenant-mgr ◄─► postgres                   │
│    │                                                  │
│    ├─► openclaw-user-a (isolated)                     │
│    ├─► openclaw-user-b (isolated)                     │
│    └─► openclaw-user-c (isolated)                     │
│                                                       │
│  各 OpenClaw 容器间无直接通信，仅通过 nginx 对外暴露    │
│  各 OpenClaw 容器可访问外网（调用 api.4tk.ai）          │
└───────────────────────────────────────────────────────┘
```

每个 OpenClaw 容器通过 Docker 网络策略限制：
- 允许出站：访问 `api.4tk.ai`（流量网关）和必要的外部服务（Telegram API 等）
- 禁止入站：仅允许 Nginx 反代的流量
- 禁止容器间互访

---

## 四、流量控制系统集成

### 4.1 集成流程

```
租户注册
   │
   ▼
租户管理服务 ──── POST /api/v1/admin/users ──────► 流量控制系统
   │              (自动创建用户 + API Key)            │
   │                                                  │
   ◄──────── 返回 API Key ◄──────────────────────────┘
   │
   ▼
生成 OpenClaw 容器配置
   │  - base_url: https://api.4tk.ai/v1
   │  - api_key: 该租户专属 Key
   │
   ▼
启动 OpenClaw 容器
```

### 4.2 OpenClaw 模型配置（指向流量网关）

每个租户容器的 `openclaw.json5` 中模型提供商配置：

```jsonc
{
  "models": {
    "providers": {
      "traffic-gateway-openai": {
        "api": "openai-completions",
        "baseUrl": "https://api.4tk.ai/v1",
        "apiKey": "${TRAFFIC_GATEWAY_API_KEY}",
        "models": [
          "gpt-4o", "gpt-4o-mini", "o3", "o1"
        ]
      },
      "traffic-gateway-anthropic": {
        "api": "anthropic-messages",
        "baseUrl": "https://api.4tk.ai",
        "apiKey": "${TRAFFIC_GATEWAY_API_KEY}",
        "models": [
          "claude-sonnet-4-5", "claude-opus-4", "claude-haiku-3-5"
        ]
      }
    },
    "default": "claude-sonnet-4-5"
  }
}
```

每个容器的 `.env` 文件：

```bash
TRAFFIC_GATEWAY_API_KEY=sk-tenant-xxxx  # 由流量控制系统自动签发
```

### 4.3 流量控制系统侧配置

在流量控制系统的管理后台中，为 OpenClaw 多租户场景创建专用 tokenGroup：

| 配置项 | 值 | 说明 |
|--------|----|------|
| tokenGroup 名称 | `openclaw-tenants` | 所有 OpenClaw 租户共用一个分组 |
| 可用模型范围 | 全模型（管理员可按需收窄） | 租户通过 OpenClaw 可调用的模型列表 |
| 单 Key 限流 | RPM: 60, TPM: 500K | 防止单租户占满资源 |
| 预算控制 | 每日 $5 / 每月 $50（可按租户调整） | 成本兜底 |

### 4.4 计费透传

- 流量控制系统为每个租户独立计费，租户可在流量控制系统控制台查看用量
- 租户管理服务定时拉取各 Key 用量数据，在 OpenClaw 管理后台聚合展示
- 余额不足时流量网关直接拒绝请求，OpenClaw 向用户返回友好提示

---

## 五、多角色（Persona）管理

### 5.1 角色定义方式

OpenClaw 通过 SKILL.md 文件定义角色。每个角色本质上是一个 Skill，包含系统提示词、行为规则和触发条件。

角色 SKILL.md 示例（`~/.openclaw/skills/personas/code-reviewer/SKILL.md`）：

```markdown
---
name: code-reviewer
description: 资深代码审查专家
trigger: /code-review
version: 1.0.0
tags: [persona, development]
author: admin
enabled: true
---

# 角色：资深代码审查专家

## 身份
你是一位拥有 15 年经验的高级软件工程师，擅长 Java/Go/TypeScript 代码审查。

## 行为准则
- 审查代码时关注：安全性、性能、可维护性、命名规范
- 对每个问题给出严重等级（Critical / Warning / Info）
- 提供修复建议和最佳实践引用
- 语言风格：专业但友好，像一位经验丰富的导师

## 输出格式
以 Markdown 表格列出发现的问题，附带修复建议代码片段。
```

### 5.2 角色管理架构

```
┌─────────────────────────────────────────────┐
│              角色模板仓库（管理员维护）        │
│                                              │
│  /data/shared/persona-templates/             │
│  ├── code-reviewer/SKILL.md                  │
│  ├── writing-assistant/SKILL.md              │
│  ├── data-analyst/SKILL.md                   │
│  ├── product-manager/SKILL.md                │
│  └── daily-assistant/SKILL.md                │
└──────────────────┬──────────────────────────┘
                   │ 安装时复制到租户目录
                   ▼
┌─────────────────────────────────────────────┐
│         租户 Persona 目录（租户私有）         │
│                                              │
│  /data/tenants/{id}/skills/personas/         │
│  ├── code-reviewer/SKILL.md    ← 从模板安装  │
│  ├── my-custom-role/SKILL.md   ← 租户自建    │
│  └── writing-helper/SKILL.md   ← 从模板安装后自定义│
└──────────────────────────────────────────────┘
```

### 5.3 角色管理功能

| 功能 | 说明 |
|------|------|
| 浏览角色模板 | 管理后台提供预置角色列表，用户搜索/浏览 |
| 一键安装 | 将模板角色复制到租户容器的 persona 目录 |
| 自定义创建 | 用户通过管理后台 UI 编写 SKILL.md 创建私有角色 |
| 编辑已安装角色 | 安装后可自由修改提示词、触发词、行为规则 |
| 启用/禁用 | 通过 `enabled: true/false` 控制角色是否激活 |
| 角色切换 | 对话中通过触发词（如 `/code-review`）切换角色 |

---

## 六、共享 Skill 池

### 6.1 Skill 池架构

```
┌───────────────────────────────────────────────────────────────┐
│                    Skill 仓库服务 (skill-registry)             │
│                                                                │
│  功能：                                                        │
│  - 存储和管理管理员开放的 Skill 集合                             │
│  - 提供搜索 API（按名称、标签、分类）                            │
│  - 提供安装 API（将 Skill 文件注入租户容器 Volume）              │
│  - 版本管理（Skill 更新时通知已安装租户）                        │
│                                                                │
│  数据存储：                                                     │
│  /data/shared/skill-pool/                                      │
│  ├── productivity/                                             │
│  │   ├── daily-report/SKILL.md                                 │
│  │   ├── meeting-summary/SKILL.md                              │
│  │   └── task-manager/SKILL.md                                 │
│  ├── development/                                              │
│  │   ├── code-review/SKILL.md                                  │
│  │   ├── git-helper/SKILL.md                                   │
│  │   └── api-tester/SKILL.md                                   │
│  ├── communication/                                            │
│  │   ├── email-drafter/SKILL.md                                │
│  │   └── translator/SKILL.md                                   │
│  ├── tools/                                                    │
│  │   ├── web-scraper/SKILL.md                                  │
│  │   └── file-converter/SKILL.md                               │
│  └── mcp-servers/                                              │
│      ├── github-mcp/config.json                                │
│      ├── postgres-mcp/config.json                              │
│      └── filesystem-mcp/config.json                            │
└───────────────────────────────────────────────────────────────┘
```

### 6.2 Skill 池功能矩阵

| 功能 | 角色 | 说明 |
|------|------|------|
| Skill 入库 | 管理员 | 从 ClawHub 社区精选或自行编写，审核后入库 |
| Skill 搜索 | 租户 | 按名称、标签、分类搜索可用 Skill |
| Skill 安装 | 租户 | 一键安装到自己的 OpenClaw 实例 |
| Skill 卸载 | 租户 | 从自己的实例移除已安装 Skill |
| Skill 自建 | 租户 | 编写私有 Skill，仅自己可见 |
| MCP Server 安装 | 租户 | 安装预配置的 MCP Server（如 GitHub、数据库等） |
| Skill 更新通知 | 系统 | Skill 池中的 Skill 更新时，通知已安装的租户 |

### 6.3 Skill 安装流程

```
用户在管理后台浏览 Skill 池
        │
        ▼
选择 Skill → 点击"安装"
        │
        ▼
skill-registry 将 SKILL.md 复制到
/data/tenants/{id}/skills/{skill-name}/
        │
        ▼
若为 MCP Server 类型 → 同时更新容器的
openclaw.json5 MCP 配置段
        │
        ▼
通知 OpenClaw 容器热加载
（写入文件后 OpenClaw 自动扫描 skills 目录）
```

---

## 七、租户管理服务（tenant-mgr）

### 7.1 核心职责

| 职责 | 说明 |
|------|------|
| 用户注册/登录 | 邮箱 + 密码注册，JWT 会话管理 |
| 租户开通 | 创建 Docker 容器 + Volume + 配置文件 + 流量网关 API Key |
| 租户注销 | 停止容器、归档数据、回收 API Key |
| 角色管理 | CRUD 角色 SKILL.md，模板浏览/安装 |
| Skill 管理 | Skill 池搜索/安装/卸载 |
| 用量查看 | 从流量控制系统拉取各租户用量和余额 |
| 容器监控 | 容器健康检查、自动重启 |

### 7.2 REST API 设计

#### 认证

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/auth/register` | 用户注册（需管理员审批或邀请码） |
| POST | `/api/v1/auth/login` | 登录，返回 JWT |
| POST | `/api/v1/auth/refresh` | JWT 续签 |

#### 租户管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/tenant/status` | 查看自己的 OpenClaw 实例状态 |
| POST | `/api/v1/tenant/restart` | 重启自己的 OpenClaw 容器 |
| GET | `/api/v1/tenant/usage` | 查看模型用量和余额 |
| GET | `/api/v1/tenant/config` | 查看当前配置 |
| PATCH | `/api/v1/tenant/config` | 更新配置（如默认模型、超时等） |

#### 角色管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/personas` | 列出已安装的角色 |
| POST | `/api/v1/personas` | 创建自定义角色 |
| PUT | `/api/v1/personas/:name` | 更新角色 |
| DELETE | `/api/v1/personas/:name` | 删除角色 |
| GET | `/api/v1/persona-templates` | 浏览角色模板库 |
| POST | `/api/v1/persona-templates/:name/install` | 从模板安装角色 |

#### Skill 池

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/skills/pool` | 搜索 Skill 池（支持分类、标签筛选） |
| GET | `/api/v1/skills/pool/:name` | 查看 Skill 详情 |
| POST | `/api/v1/skills/install/:name` | 安装 Skill 到自己的实例 |
| DELETE | `/api/v1/skills/installed/:name` | 卸载已安装 Skill |
| GET | `/api/v1/skills/installed` | 列出已安装的 Skill |

#### 管理员

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/tenants` | 查看所有租户列表 |
| POST | `/api/v1/admin/tenants/:id/approve` | 审批租户注册 |
| POST | `/api/v1/admin/tenants/:id/suspend` | 暂停租户 |
| POST | `/api/v1/admin/skill-pool` | 向 Skill 池添加新 Skill |
| DELETE | `/api/v1/admin/skill-pool/:name` | 从 Skill 池移除 Skill |
| POST | `/api/v1/admin/persona-templates` | 添加角色模板 |

### 7.3 技术选型

| 组件 | 选型 | 理由 |
|------|------|------|
| 后端框架 | Go (Gin) 或 Node.js (Fastify) | 与流量控制系统主技术栈保持一致用 Go；若追求开发效率可用 Node.js |
| 数据库 | PostgreSQL | 租户元数据、Skill 索引、审计日志 |
| 容器编排 | Docker API (dockerode/docker SDK) | 直接调用 Docker Engine API 管理容器生命周期 |
| 前端 | Vue 3 + Vite（轻量管理 UI） | 管理后台和用户 Skill/角色管理界面 |
| 认证 | JWT (access + refresh token) | 与流量控制系统保持一致 |

---

## 八、腾讯云部署方案

### 8.1 资源规划

10 个用户，每个 OpenClaw 容器约需 1 CPU + 1GB RAM，加上管理服务和基础设施：

| 资源 | 规格 | 用途 | 预估月费（元） |
|------|------|------|---------------|
| CVM 实例 | 8C16G, Ubuntu 22.04 | 主机，运行全部容器 | ~300-500 |
| 系统盘 | 100GB SSD | OS + Docker 镜像 | 含在 CVM |
| 数据盘 | 200GB SSD（可扩） | 租户数据持久化 | ~40 |
| 公网带宽 | 10Mbps（按量计费） | 用户接入 + API 出站 | ~100-200 |
| 域名 | claw.4tk.ai | 统一入口 | 已有 |
| SSL 证书 | Let's Encrypt | TLS | 免费 |

**合计预估：~500-800 元/月**

> 备选方案：若觉得 CVM 过重，可选腾讯云轻量应用服务器（Lighthouse）8C16G 套餐，月费更低且自带流量包。

### 8.2 目录结构

```
/opt/openclaw-platform/
├── docker-compose.yml              # 主编排文件
├── nginx/
│   ├── nginx.conf                  # 主配置
│   └── conf.d/
│       └── tenants.conf            # 租户路由（动态生成）
├── tenant-mgr/
│   ├── Dockerfile
│   └── src/                        # 租户管理服务源码
├── skill-registry/
│   └── src/                        # Skill 仓库服务源码
├── shared/
│   ├── skill-pool/                 # 共享 Skill 池
│   ├── persona-templates/          # 角色模板库
│   └── mcp-templates/              # MCP Server 模板
├── data/
│   └── tenants/                    # 租户隔离数据根目录
│       ├── user-alice/
│       │   ├── config/
│       │   │   ├── openclaw.json5  # OpenClaw 主配置
│       │   │   └── .env            # 凭证（API Key 等）
│       │   ├── skills/             # 已安装 Skills
│       │   │   ├── personas/       # 角色 Skills
│       │   │   └── tools/          # 工具 Skills
│       │   ├── memory/             # 持久记忆
│       │   └── workspace/          # 工作目录（文件系统访问沙箱）
│       ├── user-bob/
│       │   └── ...
│       └── user-carol/
│           └── ...
├── postgres/
│   └── data/                       # PostgreSQL 数据
└── backups/                        # 定时备份
```

### 8.3 Docker Compose 结构

```yaml
# docker-compose.yml 核心结构（示意）
version: '3.8'

services:
  nginx:
    image: nginx:alpine
    ports:
      - "443:443"
      - "80:80"
    volumes:
      - ./nginx:/etc/nginx
      - /etc/letsencrypt:/etc/letsencrypt:ro
    depends_on:
      - tenant-mgr
    restart: always

  tenant-mgr:
    build: ./tenant-mgr
    environment:
      - DATABASE_URL=postgres://...
      - TRAFFIC_GATEWAY_ADMIN_URL=https://console.4tk.ai/api/v1
      - TRAFFIC_GATEWAY_ADMIN_TOKEN=${ADMIN_TOKEN}
      - DOCKER_SOCKET=/var/run/docker.sock
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock  # 管理容器生命周期
      - ./data/tenants:/data/tenants
      - ./shared:/data/shared:ro
    depends_on:
      - postgres
    restart: always

  skill-registry:
    build: ./skill-registry
    volumes:
      - ./shared/skill-pool:/data/skill-pool
      - ./data/tenants:/data/tenants
    restart: always

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: openclaw_platform
      POSTGRES_USER: ${PG_USER}
      POSTGRES_PASSWORD: ${PG_PASSWORD}
    volumes:
      - ./postgres/data:/var/lib/postgresql/data
    restart: always

  # ---- 租户容器由 tenant-mgr 动态创建 ----
  # 不在 compose 中静态定义，通过 Docker API 动态管理
  # 模板见下文 8.4 节
```

### 8.4 租户容器启动模板

tenant-mgr 通过 Docker API 动态创建租户容器，等效命令：

```bash
docker run -d \
  --name openclaw-${TENANT_ID} \
  --network claw-network \
  --cpus 1.0 \
  --memory 1g \
  --restart unless-stopped \
  -v /opt/openclaw-platform/data/tenants/${TENANT_ID}/config:/home/claw/.openclaw/config:ro \
  -v /opt/openclaw-platform/data/tenants/${TENANT_ID}/skills:/home/claw/.openclaw/skills \
  -v /opt/openclaw-platform/data/tenants/${TENANT_ID}/memory:/home/claw/.openclaw/memory \
  -v /opt/openclaw-platform/data/tenants/${TENANT_ID}/workspace:/home/claw/workspace \
  --env-file /opt/openclaw-platform/data/tenants/${TENANT_ID}/config/.env \
  -e OPENCLAW_GATEWAY_HOST=0.0.0.0 \
  -e OPENCLAW_GATEWAY_PORT=18789 \
  openclaw/openclaw:latest
```

### 8.5 Nginx 路由配置

```nginx
# 租户路由（由 tenant-mgr 动态更新）
upstream openclaw-alice {
    server openclaw-alice:18789;
}
upstream openclaw-bob {
    server openclaw-bob:18789;
}

server {
    listen 443 ssl;
    server_name claw.4tk.ai;

    # 管理后台
    location /admin/ {
        proxy_pass http://tenant-mgr:3000/admin/;
    }

    # 租户管理 API
    location /api/ {
        proxy_pass http://tenant-mgr:3000/api/;
    }

    # 各租户 OpenClaw 网关（WebSocket + HTTP）
    location /t/alice/ {
        proxy_pass http://openclaw-alice/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }

    location /t/bob/ {
        proxy_pass http://openclaw-bob/;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

---

## 九、租户开通全流程

```
1. 管理员邀请用户（发送邀请链接）
          │
          ▼
2. 用户访问 claw.4tk.ai/register，填写邮箱+密码
          │
          ▼
3. 管理员审批（或邀请码自动审批）
          │
          ▼
4. tenant-mgr 执行开通流程：
   ├── 4a. 调用流量控制系统 API，创建用户 + 签发 API Key
   ├── 4b. 创建租户数据目录（config / skills / memory / workspace）
   ├── 4c. 生成 openclaw.json5（模型指向 api.4tk.ai + 该租户 API Key）
   ├── 4d. 复制默认 Skill 和默认角色到租户目录
   ├── 4e. 启动 Docker 容器
   └── 4f. 更新 Nginx 路由配置，reload Nginx
          │
          ▼
5. 用户收到邮件通知："您的 OpenClaw 已就绪"
   ├── Web Dashboard 地址：claw.4tk.ai/t/{username}/
   ├── Telegram Bot 接入指引
   └── Discord Bot 接入指引
          │
          ▼
6. 用户登录管理后台，可：
   ├── 浏览/安装 Skill
   ├── 浏览/安装/自建角色
   ├── 查看用量和余额
   └── 配置 MCP Server
```

---

## 十、安全策略

| 维度 | 措施 |
|------|------|
| 传输加密 | 全站 HTTPS（TLS 1.2+），HTTP 301 跳转 |
| 凭证保护 | 租户 API Key 仅存储于容器 .env 中，不落数据库；管理员 API Key 仅在 tenant-mgr 环境变量中 |
| 容器沙箱 | 每容器 `--security-opt no-new-privileges`，禁止 Docker Socket 挂载，非 root 用户运行 |
| 文件系统 | 租户 workspace 目录限制在指定路径内，禁止跨租户访问 |
| 注册管控 | 邀请制注册，非公开开放；管理员审批后方可开通 |
| API Key 安全 | 流量控制系统侧 Key 设置单 Key 限流和预算上限 |
| 审计日志 | 所有管理操作（开通/暂停/角色变更/Skill 安装）记录审计日志 |
| 数据备份 | 每日全量备份 `/data/tenants/` 和 PostgreSQL，保留 7 天 |

---

## 十一、云迁移兼容性（腾讯云 → 阿里云）

架构设计从第一天起考虑云无关性：

| 原则 | 做法 |
|------|------|
| 不依赖腾讯云专有服务 | 不使用 COS / TDSQL / CLB 等专有产品，全部用开源组件 |
| 基础设施即代码 | Docker Compose 定义全部服务，迁移时只需在新服务器 `docker compose up` |
| 数据目录独立 | 所有持久化数据在 `/opt/openclaw-platform/data/`，可直接 rsync 迁移 |
| DNS 切换 | `claw.4tk.ai` 通过 DNS 解析切换，TTL 设低（60s），切换时最多 1 分钟中断 |
| 迁移清单 | 见下方 |

### 迁移步骤清单

1. 在阿里云 ECS 上准备同规格实例（8C16G）
2. 安装 Docker + Docker Compose
3. `rsync` 同步 `/opt/openclaw-platform/` 全目录
4. 在新机器上 `docker compose up -d`
5. 验证所有容器运行正常
6. 修改 DNS 解析 `claw.4tk.ai` → 新 IP
7. 观察 1 小时，确认无异常后关停旧机器

---

## 十二、实施路线图

### P1（Week 1-2）：基础设施 + 核心链路

- [ ] 购买腾讯云 CVM / Lighthouse，初始化 Docker 环境
- [ ] 搭建 Nginx + Let's Encrypt 证书
- [ ] 开发 tenant-mgr 核心功能：注册/登录、容器创建/销毁、配置生成
- [ ] 对接流量控制系统 API：自动创建用户 + 签发 API Key
- [ ] 手动部署 2-3 个 OpenClaw 容器验证端到端流程
- [ ] 验证模型调用链路：OpenClaw → api.4tk.ai → 上游模型

### P2（Week 3-4）：Skill 池 + 角色管理

- [ ] 开发 skill-registry 服务：Skill 入库、搜索、安装
- [ ] 整理首批 Skill 池（精选 20-30 个高质量 Skill 入库）
- [ ] 开发角色模板管理：模板库浏览、安装、自定义创建
- [ ] 开发管理后台 UI（Vue 3）：用户管理、Skill 管理、角色管理
- [ ] MCP Server 模板化安装支持

### P3（Week 5-6）：运营与打磨

- [ ] 用量仪表盘：对接流量控制系统用量数据
- [ ] 自动化备份与恢复脚本
- [ ] 监控告警：容器健康、磁盘空间、用量异常
- [ ] 用户文档：接入指南、Skill 编写指南、角色定制指南
- [ ] 邀请首批用户，收集反馈迭代

---

## 十三、待决事项

| 编号 | 问题 | 影响 | 建议 |
|------|------|------|------|
| T1 | 用户通过哪些聊天渠道接入？Telegram / Discord / Web 全部支持还是首期只支持部分？ | 影响 OpenClaw 容器的 Bot 配置和凭证管理 | 首期建议 Telegram + Web Dashboard，后续扩展 |
| T2 | 每个租户是否需要独立的 Telegram Bot Token？还是共用一个 Bot + 不同 Thread？ | 影响 Bot 管理方式和用户体验 | 建议每人独立 Bot Token，隔离更彻底 |
| T3 | OpenClaw 版本更新策略：自动更新 or 管理员确认后统一更新？ | 影响运维流程和稳定性 | 建议管理员确认后统一滚动更新 |
| T4 | 是否需要限制租户的 Shell/文件系统访问能力？ | 影响安全边界 | 建议开启 SANDBOX_MODE，限制高危操作 |
| T5 | 流量控制系统的管理 API 是否已支持程序化创建用户和 API Key？ | 影响 tenant-mgr 与流量控制系统的自动化对接 | 需确认或补充接口 |
